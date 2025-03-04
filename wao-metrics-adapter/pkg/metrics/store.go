package metrics

import (
	"fmt"
	"sync"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

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

// Get returns a MetricData for the given storeKey.
// Thread-safe.
func (s *Store) Get(k storeKey) (MetricData, bool) {
	v, ok := s.m.Load(k)
	if !ok {
		return MetricData{}, false
	}
	vv, _ := v.(MetricData)
	return vv, true
}

// GetOrInit returns a MetricData for the given storeKey or inits it if not found.
// Thread-safe.
func (s *Store) GetOrInit(k storeKey) MetricData {
	if _, ok := s.m.Load(k); !ok {
		s.Set(k, MetricData{})
	}
	v, _ := s.m.Load(k)
	vv, _ := v.(MetricData)
	return vv
}

// Set sets a MetricData. Thread-safe.
func (s *Store) Set(k storeKey, m MetricData) { s.m.Store(k, m) }
