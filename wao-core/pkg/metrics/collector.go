package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type agentRunner struct {
	agent    Agent
	store    *Store
	nodeName string
	interval time.Duration
	timeout  time.Duration

	stopCh chan struct{}
}

func newAgentRunner(agent Agent, metricStore *Store, nodeName string, interval time.Duration, timeout time.Duration) *agentRunner {
	return &agentRunner{
		agent:    agent,
		store:    metricStore,
		nodeName: nodeName,
		interval: interval,
		timeout:  timeout,
		stopCh:   make(chan struct{}),
	}
}

func (r *agentRunner) Run() {
	lg := slog.With("func", "agentRunner.Run", "nodeName", r.nodeName, "agent.ValueType", r.agent.ValueType())
	lg.Info("start")
	for {
		select {
		case <-r.stopCh:
			lg.Info("stop")
			return
		case <-time.After(r.interval):
			ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
			v, err := r.agent.Fetch(ctx)
			cancel()
			if err != nil {
				lg.Error("failed to fetch", "error", err)
				continue
			}

			k := StoreKeyForNode(r.nodeName)
			m := r.store.GetOrInit(k)
			switch r.agent.ValueType() {
			case ValueInletTemperature:
				m.InletTemp = v
			case ValueDeltaPressure:
				m.DeltaPressure = v
			}
			r.store.Set(k, m)
		}
	}
}

func (r *agentRunner) Stop() { close(r.stopCh) }

type collectorKey string

func CollectorKey(objKey types.NamespacedName, valueType ValueType) collectorKey {
	return collectorKey(fmt.Sprintf("%s#%s", objKey, valueType))
}

type Collector struct{ m sync.Map }

const MinInterval = 1 * time.Second

func (c *Collector) Register(k collectorKey, a Agent, s *Store, nodeName string, interval time.Duration, timeout time.Duration) {
	lg := slog.With("func", "Collector.Register", "key", k, "nodeName", nodeName)
	lg.Info("register")

	if interval < MinInterval {
		interval = MinInterval
	}
	ar := newAgentRunner(a, s, nodeName, interval, timeout)
	go ar.Run()
	c.m.Store(k, ar)
}

func (c *Collector) Unregister(k collectorKey) {
	lg := slog.With("func", "Collector.Unregister", "key", k)
	lg.Info("unregister")

	defer c.m.Delete(k)

	v, ok := c.m.Load(k)
	if !ok {
		return
	}

	ar, ok := v.(agentRunner)
	if !ok {
		return
	}

	ar.Stop()
}
