package metrics

import (
	"context"
	"fmt"
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
	for {
		select {
		case <-r.stopCh:
			return
		case <-time.After(r.interval):
			ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
			v, err := r.agent.Fetch(ctx)
			cancel()
			if err != nil {
				// TODO: notify error?
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
	if interval < MinInterval {
		interval = MinInterval
	}
	ar := newAgentRunner(a, s, nodeName, interval, timeout)
	go ar.Run()
	c.m.Store(k, ar)
}

func (c *Collector) Unregister(k collectorKey) {
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
