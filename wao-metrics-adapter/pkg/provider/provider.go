package provider

import (
	"context"
	"fmt"
	"math"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider/defaults"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider/helpers"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metric"
)

type Provider struct {
	defaults.DefaultCustomMetricsProvider
	// defaults.DefaultExternalMetricsProvider

	client dynamic.Interface
	mapper apimeta.RESTMapper

	metricStore *metric.Store
}

var (
	_ provider.CustomMetricsProvider = (*Provider)(nil)
	// _ provider.ExternalMetricsProvider = (*Provider)(nil)
)

func New(client dynamic.Interface, mapper apimeta.RESTMapper, metricStore *metric.Store) *Provider {
	return &Provider{
		client:      client,
		mapper:      mapper,
		metricStore: metricStore,
	}
}

var (
	grNode       = schema.GroupResource{Group: "", Resource: "nodes"}
	supportedGRs = []schema.GroupResource{grNode}
)

// validateResource rejects unsupported GroupResources and returns normalized CustomMetricInfo
func (p *Provider) validateResource(namespace, name string, info provider.CustomMetricInfo) (provider.CustomMetricInfo, error) {
	// NOTE: normalized to lowercase, plural.
	// /apis/custom.metrics.k8s.io/v1beta2/namespaces/ns0/pods/pod0/fuga
	// provider.CustomMetricInfo{GroupResource:schema.GroupResource{Group:"", Resource:"pods"}, Namespaced:true, Metric:"fuga"}
	// /apis/custom.metrics.k8s.io/v1beta2/node/node0/fuga
	// provider.CustomMetricInfo{GroupResource:schema.GroupResource{Group:"", Resource:"nodes"}, Namespaced:false, Metric:"fuga"}
	// /apis/custom.metrics.k8s.io/v1beta2/namespaces/ns0/deployments.apps/deploy0/fuga
	// provider.CustomMetricInfo{GroupResource:schema.GroupResource{Group:"apps", Resource:"deployments"}, Namespaced:true, Metric:"fuga"}
	info, _, err := info.Normalized(p.mapper)
	if err != nil {
		return provider.CustomMetricInfo{}, err
	}

	var supported bool
	for _, gr := range supportedGRs {
		if info.GroupResource == gr {
			supported = true
		}
	}
	if !supported {
		return provider.CustomMetricInfo{}, fmt.Errorf("unsupported GroupResource: %v", info.GroupResource)
	}

	return info, nil
}

// fixedScale converts values. (123.456789, 3)->(123.456, -3)
func fixedScale(r float64, precision int32) (n int64, scale int32) {
	x := math.Pow10(int(precision))
	n = int64(r * x)
	scale = -precision
	return
}

func metricValueMilli(objRef custom_metrics.ObjectReference, t time.Time, key string, value int64) *custom_metrics.MetricValue {
	var window int64 = 0
	return &custom_metrics.MetricValue{
		DescribedObject: objRef,
		Metric:          custom_metrics.MetricIdentifier{Name: key},
		Timestamp:       metav1.Time{Time: t},
		WindowSeconds:   &window,
		Value:           *resource.NewMilliQuantity(value, resource.DecimalSI),
	}
}

func metricValueScale(objRef custom_metrics.ObjectReference, t time.Time, key string, value int64, scale int32) *custom_metrics.MetricValue {
	var window int64 = 0
	return &custom_metrics.MetricValue{
		DescribedObject: objRef,
		Metric:          custom_metrics.MetricIdentifier{Name: key},
		Timestamp:       metav1.Time{Time: t},
		WindowSeconds:   &window,
		Value:           *resource.NewScaledQuantity(value, resource.Scale(scale)),
	}
}

// metricFor constructs a result for a single metric value.
func (p *Provider) metricFor(namespace, name string, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	// get value
	info, err := p.validateResource(namespace, name, info)
	if err != nil {
		return nil, err
	}
	k := metric.StoreKey(namespace, name, info)
	m := p.metricStore.Get(k)

	// construct objref
	objRef, err := helpers.ReferenceFor(p.mapper, types.NamespacedName{Namespace: namespace, Name: name}, info)
	if err != nil {
		return nil, err
	}

	switch info.Metric {
	case metric.ValueInletTemperature:
		v, s := fixedScale(m.InletTemp, 6)
		return metricValueScale(objRef, time.Now(), info.Metric, v, s), nil
	case metric.ValueDeltaPressure:
		v, s := fixedScale(m.DeltaPressure, 6)
		return metricValueScale(objRef, time.Now(), info.Metric, v, s), nil
	default:
		return nil, fmt.Errorf("metric not supported: name=%s", info.Metric)
	}
}

func (p *Provider) GetMetricByName(ctx context.Context, name types.NamespacedName, info provider.CustomMetricInfo, _ labels.Selector) (*custom_metrics.MetricValue, error) {
	return p.metricFor(name.Namespace, name.Name, info)
}

func (p *Provider) GetMetricBySelector(ctx context.Context, namespace string, selector labels.Selector, info provider.CustomMetricInfo, _ labels.Selector) (*custom_metrics.MetricValueList, error) {
	names, err := helpers.ListObjectNames(p.mapper, p.client, namespace, selector, info)
	if err != nil {
		return nil, err
	}

	res := make([]custom_metrics.MetricValue, len(names))
	for i, name := range names {
		value, err := p.metricFor(namespace, name, info)
		if err != nil {
			return nil, err
		}
		res[i] = *value
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}
