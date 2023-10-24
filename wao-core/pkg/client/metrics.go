package client

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	custommetricsv1beta2 "k8s.io/metrics/pkg/apis/custom_metrics/v1beta2"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"

	"github.com/waok8s/wao-core/pkg/metrics"
)

type CachedMetricsClient struct {
	metricsclientset    metricsclientv1beta1.MetricsV1beta1Interface
	custommetricsclient custommetricsclient.CustomMetricsClient

	ttl   time.Duration
	cache sync.Map

	logWriter io.Writer
}

func NewCachedMetricsClient(metricsclientset metricsclientv1beta1.MetricsV1beta1Interface, custommetricsclient custommetricsclient.CustomMetricsClient, ttl time.Duration, logWriter io.Writer) *CachedMetricsClient {
	if logWriter == nil {
		logWriter = io.Discard
	}
	return &CachedMetricsClient{
		metricsclientset:    metricsclientset,
		custommetricsclient: custommetricsclient,
		ttl:                 ttl,
		logWriter:           logWriter,
	}
}

func metricsCacheKey(obj types.NamespacedName, metricType, metricName string) string {
	return fmt.Sprintf("%s#%s#%s", obj.String(), metricType, metricName)
}

const (
	metricTypeNode = "node"
	metricTypePod  = "pod"

	metricNameMetrics   = "metrics"
	metricNameInletTemp = metrics.ValueInletTemperature
	metricNameDeltaP    = metrics.ValueDeltaPressure
)

type metricsCache struct {
	NodeMetrics   *metricsv1beta1.NodeMetrics
	PodMetrics    *metricsv1beta1.PodMetrics
	CustomMetrics map[string]*custommetricsv1beta2.MetricValue

	ExpiredAt time.Time
}

func (c *CachedMetricsClient) get(ctx context.Context, obj types.NamespacedName, metricType string, metricName string) (*metricsCache, error) {

	key := metricsCacheKey(obj, metricType, metricName)

	if v, ok1 := c.cache.Load(key); ok1 {
		if cv, ok2 := v.(*metricsCache); ok2 {
			if cv.ExpiredAt.After(time.Now()) {
				fmt.Fprintf(c.logWriter, "metrics cache hit key=%s\n", key)
				return cv, nil
			}
		}
	}
	fmt.Fprintf(c.logWriter, "metrics cache missed key=%s\n", key)

	cv := &metricsCache{
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
			cv.NodeMetrics = nodeMetrics
		case metricNameInletTemp, metricNameDeltaP:
			metricValue, err := c.custommetricsclient.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, obj.Name, metricName, labels.NewSelector())
			if err != nil {
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			cv.CustomMetrics[metricName] = metricValue
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
			cv.PodMetrics = podMetrics
		default:
			return nil, fmt.Errorf("unknown metricName=%s for metricType=%s", metricName, metricType)
		}
	default:
		return nil, fmt.Errorf("unknown metricType=%s", metricType)
	}

	c.cache.Store(key, cv)

	return cv, nil
}

func (c *CachedMetricsClient) GetNodeMetrics(ctx context.Context, name string) (*metricsv1beta1.NodeMetrics, error) {
	cv, err := c.get(ctx, types.NamespacedName{Name: name}, metricTypeNode, metricNameMetrics)
	if err != nil {
		return nil, err
	}
	return cv.NodeMetrics, nil
}

func (c *CachedMetricsClient) GetPodMetrics(ctx context.Context, namespace string, name string) (*metricsv1beta1.PodMetrics, error) {
	cv, err := c.get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, metricTypePod, metricNameMetrics)
	if err != nil {
		return nil, err
	}
	return cv.PodMetrics, nil
}

func (c *CachedMetricsClient) GetCustomMetricForNode(ctx context.Context, name string, metricName string) (*custommetricsv1beta2.MetricValue, error) {
	cv, err := c.get(ctx, types.NamespacedName{Name: name}, metricTypeNode, metricName)
	if err != nil {
		return nil, err
	}
	return cv.CustomMetrics[metricName], nil
}
