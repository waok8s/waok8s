package minimizepower

import (
	"context"
	"fmt"
	"sync"
	"time"

	waometric "github.com/waok8s/wao-metrics-adapter/pkg/metric"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	custommetricsv1beta2 "k8s.io/metrics/pkg/apis/custom_metrics/v1beta2"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"
)

type MetricsCache struct {
	metricsclientset    metricsclientv1beta1.MetricsV1beta1Interface
	custommetricsclient custommetricsclient.CustomMetricsClient

	ttl   time.Duration
	cache sync.Map
}

func NewMetricsCache(metricsclientset metricsclientv1beta1.MetricsV1beta1Interface, custommetricsclient custommetricsclient.CustomMetricsClient, expiration time.Duration) *MetricsCache {
	return &MetricsCache{
		metricsclientset:    metricsclientset,
		custommetricsclient: custommetricsclient,
		ttl:                 expiration,
	}
}

func cacheKey(obj types.NamespacedName, metricType, metricName string) string {
	return fmt.Sprintf("%s#%s#%s", obj.String(), metricType, metricName)
}

const (
	metricTypeNode = "node"
	metricTypePod  = "pod"

	metricNameMetrics   = "metrics"
	metricNameInletTemp = waometric.ValueInletTemperature
	metricNameDeltaP    = waometric.ValueDeltaPressure
)

type cachedObject struct {
	NodeMetrics   *metricsv1beta1.NodeMetrics
	PodMetrics    *metricsv1beta1.PodMetrics
	CustomMetrics map[string]*custommetricsv1beta2.MetricValue

	ExpiredAt time.Time
}

func (c *MetricsCache) get(ctx context.Context, obj types.NamespacedName, metricType string, metricName string) (*cachedObject, error) {

	key := cacheKey(obj, metricType, metricName)

	if v, ok1 := c.cache.Load(key); ok1 {
		if co, ok2 := v.(*cachedObject); ok2 {
			if co.ExpiredAt.After(time.Now()) {
				return co, nil
			}
		}
	}

	co := &cachedObject{
		CustomMetrics: make(map[string]*custommetricsv1beta2.MetricValue),
		ExpiredAt:     time.Now().Add(c.ttl),
	}

	switch metricType {
	case metricTypeNode:
		switch metricName {
		case metricNameMetrics:
			nodeMetrics, err := c.metricsclientset.NodeMetricses().Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			co.NodeMetrics = nodeMetrics
		case metricNameInletTemp, metricNameDeltaP:
			metricValue, err := c.custommetricsclient.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, obj.Name, metricName, labels.NewSelector())
			if err != nil {
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			co.CustomMetrics[metricName] = metricValue
		default:
			return nil, fmt.Errorf("unknown metricName=%s for metricType=%s", metricName, metricType)
		}
	case metricTypePod:
		switch metricName {
		case metricNameMetrics:
			podMetrics, err := c.metricsclientset.PodMetricses(obj.Namespace).Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			co.PodMetrics = podMetrics
		default:
			return nil, fmt.Errorf("unknown metricName=%s for metricType=%s", metricName, metricType)
		}
	default:
		return nil, fmt.Errorf("unknown metricType=%s", metricType)
	}

	c.cache.Store(key, co)

	return co, nil
}

func (c *MetricsCache) GetNodeMetrics(ctx context.Context, name string) (*metricsv1beta1.NodeMetrics, error) {
	co, err := c.get(ctx, types.NamespacedName{Name: name}, metricTypeNode, metricNameMetrics)
	if err != nil {
		return nil, err
	}
	return co.NodeMetrics, nil
}

func (c *MetricsCache) GetPodMetrics(ctx context.Context, namespace string, name string) (*metricsv1beta1.PodMetrics, error) {
	co, err := c.get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, metricTypePod, metricNameMetrics)
	if err != nil {
		return nil, err
	}
	return co.PodMetrics, nil
}

func (c *MetricsCache) GetCustomMetricForNode(ctx context.Context, name string, metricName string) (*custommetricsv1beta2.MetricValue, error) {
	co, err := c.get(ctx, types.NamespacedName{Name: name}, metricTypeNode, metricName)
	if err != nil {
		return nil, err
	}
	return co.CustomMetrics[metricName], nil
}
