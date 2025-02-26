package client

import (
	"context"
	"fmt"
	"log/slog"
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
}

func NewCachedMetricsClient(metricsclientset metricsclientv1beta1.MetricsV1beta1Interface, custommetricsclient custommetricsclient.CustomMetricsClient, ttl time.Duration) *CachedMetricsClient {
	return &CachedMetricsClient{
		metricsclientset:    metricsclientset,
		custommetricsclient: custommetricsclient,
		ttl:                 ttl,
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

	// mu is used to avoid concurrent requests for the same key (which would result in multiple requests to origin servers)
	mu sync.Mutex
}

func (c *CachedMetricsClient) get(ctx context.Context, obj types.NamespacedName, metricType string, metricName string) (*metricsCache, error) {

	key := metricsCacheKey(obj, metricType, metricName)
	lg := slog.With("func", "CachedMetricsClient.get", "key", key)

	if v, ok1 := c.cache.Load(key); ok1 {
		if cv, ok2 := v.(*metricsCache); ok2 {

			// Wait until the cache is ready
			cv.mu.Lock()
			lg.Debug("metrics cache is available")
			cv.mu.Unlock() // NOTE: any better way to do this?

			// Check if the cache is expired
			if cv.ExpiredAt.After(time.Now()) {
				lg.Debug("metrics cache hit")
				return cv, nil
			}
		}
	}
	lg.Debug("metrics cache missed")

	// Push an empty cache and lock it to avoid concurrent requests
	cv := &metricsCache{
		CustomMetrics: make(map[string]*custommetricsv1beta2.MetricValue),
		ExpiredAt:     time.Now().Add(c.ttl),
	}
	cv.mu.Lock()
	c.cache.Store(key, cv)

	switch metricType {
	case metricTypeNode:
		switch metricName {
		case metricNameMetrics:
			nodeMetrics, err := c.metricsclientset.NodeMetricses().Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				cv.mu.Unlock()
				c.cache.Delete(key)
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			cv.NodeMetrics = nodeMetrics
		case metricNameInletTemp, metricNameDeltaP:
			metricValue, err := c.custommetricsclient.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, obj.Name, metricName, labels.NewSelector())
			if err != nil {
				cv.mu.Unlock()
				c.cache.Delete(key)
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			cv.CustomMetrics[metricName] = metricValue
		default:
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, fmt.Errorf("unknown metricName=%s for metricType=%s", metricName, metricType)
		}
	case metricTypePod:
		switch metricName {
		case metricNameMetrics:
			podMetrics, err := c.metricsclientset.PodMetricses(obj.Namespace).Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				cv.mu.Unlock()
				c.cache.Delete(key)
				return nil, fmt.Errorf("unable to get metrics for obj=%s metricType=%s metricName=%s: %w", obj, metricType, metricName, err)
			}
			cv.PodMetrics = podMetrics
		default:
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, fmt.Errorf("unknown metricName=%s for metricType=%s", metricName, metricType)
		}
	default:
		cv.mu.Unlock()
		c.cache.Delete(key)
		return nil, fmt.Errorf("unknown metricType=%s", metricType)
	}

	c.cache.Store(key, cv)
	cv.mu.Unlock()

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
