package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metric"
	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector"
)

type metricCollectorRunner struct {
	collector   metriccollector.MetricCollector
	metricStore *metric.Store
	nodeName    string
	interval    time.Duration
	timeout     time.Duration

	stopCh chan struct{}
}

func newMetricCollectorRunner(collector metriccollector.MetricCollector, metricStore *metric.Store, nodeName string, interval time.Duration, timeout time.Duration) *metricCollectorRunner {
	return &metricCollectorRunner{
		collector:   collector,
		metricStore: metricStore,
		nodeName:    nodeName,
		interval:    interval,
		timeout:     timeout,
		stopCh:      make(chan struct{}),
	}
}

func (r *metricCollectorRunner) Run() {
	for {
		select {
		case <-r.stopCh:
			return
		case <-time.After(r.interval):
			ctx, cf := context.WithTimeout(context.Background(), r.timeout)
			v, err := r.collector.Fetch(ctx)
			if err != nil {
				// TODO: log
			}

			k := metric.StoreKeyForNode(r.nodeName)
			m := r.metricStore.Get(k)
			switch r.collector.ValueType() {
			case metric.ValueInletTemperature:
				m.InletTemp = v
			case metric.ValueDeltaPressure:
				m.DeltaPressure = v
			}
			r.metricStore.Set(k, m)

			cf()
		}
	}
}

func (r *metricCollectorRunner) Stop() { close(r.stopCh) }

type registryKey string

func RegistryKey(objKey types.NamespacedName, valueType metric.ValueType) registryKey {
	return registryKey(fmt.Sprintf("%s#%s", objKey, valueType))
}

type Registry struct{ m sync.Map }

const MinInterval = 1 * time.Second

func (r *Registry) Register(k registryKey, c metriccollector.MetricCollector, s *metric.Store, nodeName string, interval time.Duration, timeout time.Duration) {
	if interval < MinInterval {
		interval = MinInterval
	}
	cr := newMetricCollectorRunner(c, s, nodeName, interval, timeout)
	go cr.Run()
	r.m.Store(k, cr)
}

func (r *Registry) Unregister(k registryKey) {
	defer r.m.Delete(k)

	v, ok := r.m.Load(k)
	if !ok {
		return
	}

	cr, ok := v.(metricCollectorRunner)
	if !ok {
		return
	}

	cr.Stop()
}
