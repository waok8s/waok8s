package metric

import (
	"fmt"
	"sync"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

type ValueType string

const (
	ValueInletTemperature = "inlet_temp"
	ValueDeltaPressure    = "delta_p"
)

type Metric struct {
	InletTemp     float64
	DeltaPressure float64
}

type storeKey string

// StoreKey constructs a storeKey.
//
// Format: {namespace}/{resource.group}/{name}
// Examples:
//   - Pod: "default/pods/pod0"
//   - Node: "/nodes/node0"
//   - Deployment: "default/deployments.apps/deploy0"
//
// NOTE: CustomMetricInfo should be normalized with CustomMetricInfo.Normalized().
func StoreKey(namespace, name string, info provider.CustomMetricInfo) storeKey {
	return storeKey(fmt.Sprintf("%s/%s/%s", namespace, info.GroupResource.String(), name))
}

// StoreKeyForNode constructs a storeKey for the given node name.
//
// Format: nodes/{name}
func StoreKeyForNode(name string) storeKey {
	return storeKey(fmt.Sprintf("/nodes/%s", name))
}

type Store struct{ m sync.Map }

// Get returns a Metric for the given storeKey or inits it if not found.
// Thread-safe.
func (s *Store) Get(k storeKey) Metric {
	if _, ok := s.m.Load(k); !ok {
		s.Set(k, Metric{})
	}
	v, _ := s.m.Load(k)
	vv, _ := v.(Metric)
	return vv
}

// Set sets a Metric. Thread-safe.
func (s *Store) Set(k storeKey, m Metric) { s.m.Store(k, m) }
