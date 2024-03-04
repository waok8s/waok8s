package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
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

var agentRunnerMaxInitialDelay = 10 * time.Second

func (r *agentRunner) Run() {
	lg := slog.With("func", "agentRunner.Run", "nodeName", r.nodeName, "agent.ValueType", r.agent.ValueType())

	// random sleep to avoid spikes
	d := time.Duration(rand.Int63n(int64(min(r.interval, agentRunnerMaxInitialDelay))))
	lg.Info("start with initial delay", "delay", d)
	time.Sleep(d)

	for {
		select {
		case <-r.stopCh:
			lg.Info("stopped")
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
				m.InletTempTimestamp = time.Now()
			case ValueDeltaPressure:
				m.DeltaPressure = v
				m.DeltaPressureTimestamp = time.Now()
			}
			r.store.Set(k, m)
		}
	}
}

func (r *agentRunner) Stop() {
	lg := slog.With("func", "agentRunner.Stop", "nodeName", r.nodeName, "agent.ValueType", r.agent.ValueType())
	lg.Info("stop")
	close(r.stopCh)
}

type collectorKey string

func CollectorKey(objKey types.NamespacedName, valueType ValueType) collectorKey {
	return collectorKey(fmt.Sprintf("%s#%s", objKey, valueType))
}

type Collector struct{ m sync.Map }

const MinInterval = 1 * time.Second

func (c *Collector) Register(k collectorKey, a Agent, s *Store, nodeName string, interval time.Duration, timeout time.Duration) {
	lg := slog.With("func", "Collector.Register", "key", k, "nodeName", nodeName)
	lg.Info("register")

	ar := newAgentRunner(a, s, nodeName, max(interval, MinInterval), timeout)
	go ar.Run()
	c.m.Store(k, ar)
}

func (c *Collector) Unregister(k collectorKey) {
	lg := slog.With("func", "Collector.Unregister", "key", k)
	lg.Info("unregister")

	defer c.m.Delete(k)

	v, ok := c.m.Load(k)
	if !ok {
		lg.Error("agentRunner not found")
		return
	}

	ar, ok := v.(*agentRunner)
	if !ok {
		lg.Error("agentRunner type assertion failed")
		return
	}

	ar.Stop()
}
