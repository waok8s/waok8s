package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
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

	waometrics "github.com/waok8s/wao-core/pkg/metrics"
)

var (
	MetricTTL = 60 * time.Second
)

type Provider struct {
	defaults.DefaultCustomMetricsProvider
	// defaults.DefaultExternalMetricsProvider

	client dynamic.Interface
	mapper apimeta.RESTMapper

	metricsStore *waometrics.Store
}

var (
	_ provider.CustomMetricsProvider = (*Provider)(nil)
	// _ provider.ExternalMetricsProvider = (*Provider)(nil)
)

func New(client dynamic.Interface, mapper apimeta.RESTMapper, metricStore *waometrics.Store) *Provider {
	return &Provider{
		client:       client,
		mapper:       mapper,
		metricsStore: metricStore,
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
// The `error` return value is ensured to be in the metav1.Status format.
func (p *Provider) metricFor(namespace, name string, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	// get value
	info, err := p.validateResource(namespace, name, info)
	if err != nil {
		return nil, apierr.NewBadRequest(err.Error())
	}
	k := waometrics.StoreKey(namespace, name, info)
	m, ok := p.metricsStore.Get(k)
	if !ok {
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, types.NamespacedName{Namespace: namespace, Name: name}.String())
	}

	// check timestamp
	switch info.Metric {
	case waometrics.ValueInletTemperature:
		if m.InletTempTimestamp.Add(MetricTTL).Before(time.Now()) {
			return nil, newMetricExpiredForError(info.GroupResource, info.Metric, types.NamespacedName{Namespace: namespace, Name: name}.String())
		}
	case waometrics.ValueDeltaPressure:
		if m.DeltaPressureTimestamp.Add(MetricTTL).Before(time.Now()) {
			return nil, newMetricExpiredForError(info.GroupResource, info.Metric, types.NamespacedName{Namespace: namespace, Name: name}.String())
		}
	default:
		return nil, apierr.NewInternalError(fmt.Errorf("metric not supported: name=%s", info.Metric))
	}

	// construct result
	objRef, err := helpers.ReferenceFor(p.mapper, types.NamespacedName{Namespace: namespace, Name: name}, info)
	if err != nil {
		return nil, apierr.NewInternalError(err)
	}
	switch info.Metric {
	case waometrics.ValueInletTemperature:
		v, s := fixedScale(m.InletTemp, 6)
		return metricValueScale(objRef, time.Now(), info.Metric, v, s), nil
	case waometrics.ValueDeltaPressure:
		v, s := fixedScale(m.DeltaPressure, 6)
		return metricValueScale(objRef, time.Now(), info.Metric, v, s), nil
	default:
		return nil, apierr.NewInternalError(fmt.Errorf("metric not supported: name=%s", info.Metric))
	}
}

// GetMetricByName implements CustomMetricsProvider interface.
// The `error` return value is ensured to be in the metav1.Status format.
func (p *Provider) GetMetricByName(ctx context.Context, name types.NamespacedName, info provider.CustomMetricInfo, _ labels.Selector) (*custom_metrics.MetricValue, error) {
	return p.metricFor(name.Namespace, name.Name, info)
}

// GetMetricBySelector implements CustomMetricsProvider interface.
// The `error` return value is ensured to be in the metav1.Status format.
func (p *Provider) GetMetricBySelector(ctx context.Context, namespace string, selector labels.Selector, info provider.CustomMetricInfo, _ labels.Selector) (*custom_metrics.MetricValueList, error) {
	names, err := helpers.ListObjectNames(p.mapper, p.client, namespace, selector, info)
	if err != nil {
		return nil, apierr.NewInternalError(fmt.Errorf("failed to list objects: %w", err))
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

// newMetricExpiredForError returns a StatusError, specialized for the case where a metric is expired.
func newMetricExpiredForError(resource schema.GroupResource, metricName string, resourceName string) *apierr.StatusError {
	return &apierr.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    int32(http.StatusNotFound),
		Reason:  metav1.StatusReasonNotFound,
		Message: fmt.Sprintf("metric %s for %s %s is expired", metricName, resource.String(), resourceName),
	}}
}
