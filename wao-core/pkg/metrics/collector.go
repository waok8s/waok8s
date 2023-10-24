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

func newAgentRunner(collector Agent, metricStore *Store, nodeName string, interval time.Duration, timeout time.Duration) *agentRunner {
	return &agentRunner{
		agent:    collector,
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
			ctx, cf := context.WithTimeout(context.Background(), r.timeout)
			v, err := r.agent.Fetch(ctx)
			if err != nil {
				// TODO: log
			}

			k := StoreKeyForNode(r.nodeName)
			m := r.store.Get(k)
			switch r.agent.ValueType() {
			case ValueInletTemperature:
				m.InletTemp = v
			case ValueDeltaPressure:
				m.DeltaPressure = v
			}
			r.store.Set(k, m)

			cf()
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

func (r *Collector) Register(k collectorKey, c Agent, s *Store, nodeName string, interval time.Duration, timeout time.Duration) {
	if interval < MinInterval {
		interval = MinInterval
	}
	cr := newAgentRunner(c, s, nodeName, interval, timeout)
	go cr.Run()
	r.m.Store(k, cr)
}

func (r *Collector) Unregister(k collectorKey) {
	defer r.m.Delete(k)

	v, ok := r.m.Load(k)
	if !ok {
		return
	}

	cr, ok := v.(agentRunner)
	if !ok {
		return
	}

	cr.Stop()
}
