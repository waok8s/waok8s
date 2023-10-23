package minimizepower

import (
	"context"
	"fmt"
	"math"
	"time"

	waometric "github.com/waok8s/wao-metrics-adapter/pkg/metric"
	"github.com/waok8s/wao-scheduler/pkg/predictor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"
)

const (
	// AssumedCPUUsageRate sets Pod CPU usage to (limits.cpu * AssumedCPUUsageRate) if requests.cpu is empty.
	AssumedCPUUsageRate = 0.85
	// LowerLimitCPUUsageRate sets the lowest Pod CPU usage to (limits.cpu * LowerLimitCPUUsageRate) for the score calculation.
	LowerLimitCPUUsageRate = 0.50

	// MetricsCacheTTL is the expiration time of the metrics cache.
	MetricsCacheTTL = 15 * time.Second
	// PredictorCacheTTL is the expiration time of the predictor cache.
	PredictorCacheTTL = 15 * time.Second
)

type MinimizePower struct {
	clientset            kubernetes.Interface
	metricsclientset     metricsclientv1beta1.MetricsV1beta1Interface
	custommetricsclient  custommetricsclient.CustomMetricsClient
	metricscache         *MetricsCache
	snapshotSharedLister framework.SharedLister
}

var _ framework.ScorePlugin = (*MinimizePower)(nil)
var _ framework.ScoreExtensions = (*MinimizePower)(nil)

var (
	Name = "MinimizePower"
)

// New initializes a new plugin and returns it.
func New(_ runtime.Object, fh framework.Handle) (framework.Plugin, error) {

	cfg := fh.KubeConfig()

	// init metrics client
	mc, err := metricsclientv1beta1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	// init custom metrics client
	// https://github.com/kubernetes/kubernetes/blob/7b9d244efd19f0d4cce4f46d1f34a6c7cff97b18/test/e2e/instrumentation/monitoring/custom_metrics_stackdriver.go#L59
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	rm := restmapper.NewDeferredDiscoveryRESTMapper(cacheddiscovery.NewMemCacheClient(dc))
	rm.Reset()
	avg := custommetricsclient.NewAvailableAPIsGetter(dc)
	cmc := custommetricsclient.NewForConfig(cfg, rm, avg)

	return &MinimizePower{
		clientset:            fh.ClientSet(),
		metricsclientset:     mc,
		custommetricsclient:  cmc,
		metricscache:         NewMetricsCache(mc, cmc, MetricsCacheTTL),
		snapshotSharedLister: fh.SnapshotSharedLister(),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (*MinimizePower) Name() string {
	return Name
}

// ScoreExtensions returns a ScoreExtensions interface.
func (pl *MinimizePower) ScoreExtensions() framework.ScoreExtensions { return pl }

// Score returns how many watts will be increased by the given pod (lower is better).
//
// This function never returns an error (as errors cause the pod to be rejected).
// If an error occurs, it is logged and the score is set to math.MaxInt64.
func (pl *MinimizePower) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	klog.InfoS("MinimizePower.Score", "node", nodeName, "pod", pod.Name)

	nodeInfo, err := pl.snapshotSharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// get node and node metrics
	node := nodeInfo.Node()
	// nodeMetrics, err := pl.metricsclientset.NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{}) // non-cached
	nodeMetrics, err := pl.metricscache.GetNodeMetrics(ctx, node.Name) // cached
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// get pods and pods metrics, ignore pods that cannot get metrics
	var nodePods []*corev1.Pod
	var nodePodsMetricses []*metricsv1beta1.PodMetrics
	for i, p := range nodeInfo.Pods {
		// podMetrics, err := pl.metricsclientset.PodMetricses(p.Pod.Namespace).Get(ctx, p.Pod.Name, metav1.GetOptions{}) // non-cached
		podMetrics, err := pl.metricscache.GetPodMetrics(ctx, p.Pod.Namespace, p.Pod.Name) // cached
		if err != nil {
			klog.ErrorS(err, "MinimizePower.Score skip pod as error occurred", "node", nodeName, "pod", pod.Name, "nodeInfo.Pods[i]", i)
			continue
		}
		nodePods = append(nodePods, p.Pod)
		nodePodsMetricses = append(nodePodsMetricses, podMetrics)
	}

	beforeUsage, afterUsage := pl.AssumedCPUUsage(ctx, node, nodeMetrics, nodePods, nodePodsMetricses, pod, AssumedCPUUsageRate, LowerLimitCPUUsageRate)
	if beforeUsage == afterUsage { // Both requests.cpu and limits.cpu are empty or zero. Normally, this should not happen.
		klog.ErrorS(fmt.Errorf("beforeUsage == afterUsage v=%v", beforeUsage), "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	if afterUsage > 1 { // CPU overcommitment, make the node lowest priority.
		klog.InfoS("MinimizePower.Score score=MaxInt64 as CPU overcommitment", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// get custom metrics
	// inletTemp, err := pl.custommetricsclient.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, nodeName, waometric.ValueInletTemperature, labels.NewSelector()) // non-cached
	inletTemp, err := pl.metricscache.GetCustomMetricForNode(ctx, nodeName, waometric.ValueInletTemperature) // cached
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=0 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	// deltaP, err := pl.custommetricsclient.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, nodeName, waometric.ValueDeltaPressure, labels.NewSelector()) // non-cached
	deltaP, err := pl.metricscache.GetCustomMetricForNode(ctx, nodeName, waometric.ValueDeltaPressure) // cached
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=0 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	var pcp predictor.PowerConsumptionPredictor // TODO: init predictor
	beforeWatt, err := pcp.Predict(ctx, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=0 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	afterWatt, err := pcp.Predict(ctx, afterUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=0 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	podPowerConsumption := int64(afterWatt - beforeWatt)
	if podPowerConsumption < 0 {
		klog.InfoS("MinimizePower.Score round podPowerConsumption to 0", "node", nodeName, "pod", pod.Name, "podPowerConsumption", podPowerConsumption)
		podPowerConsumption = 0
	}

	return podPowerConsumption, nil
}

func (pl *MinimizePower) NormalizeScore(_ context.Context, _ *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	klog.InfoS("MinimizePower.NormalizeScore before", "pod", pod.Name, "scores", scores)

	PowerConsumptions2Scores(scores)

	klog.InfoS("MinimizePower.NormalizeScore after", "pod", pod.Name, "scores", scores)

	return nil
}

func PowerConsumptions2Scores(scores framework.NodeScoreList) {
	highest := int64(math.MinInt64)
	lowest := int64(math.MaxInt64)

	for _, score := range scores {
		if score.Score > highest {
			highest = score.Score
		}
		if score.Score < lowest {
			lowest = score.Score
		}
	}

	for node, score := range scores {
		if highest != lowest {
			scores[node].Score = int64(framework.MaxNodeScore - (framework.MaxNodeScore * (score.Score - lowest) / (highest - lowest)))
		} else {
			scores[node].Score = 0
		}
	}
}

// AssumedCPUUsage assumes the CPU usage increment by allocating the given Pod.
//
//   - Node CPU usage may be greater than 1.0.
//   - Containers are ignored when it has empty requests.cpu and empty limit.cpu.
//   - assumedCPUUsageRate is used when a container has empty requests.cpu and non-empty limits.cpu.
//   - lowerLimitCPUUsageRate is the lower limit CPU usage rate of a pod.
func (pl *MinimizePower) AssumedCPUUsage(ctx context.Context,
	node *corev1.Node, nodeMetrics *metricsv1beta1.NodeMetrics,
	nodePods []*corev1.Pod, nodePodsMetricses []*metricsv1beta1.PodMetrics,
	pod *corev1.Pod,
	assumedCPUUsageRate float64, lowerLimitCPUUsageRate float64,
) (before float64, after float64) {

	nodeCPUUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()
	nodeCPUAllocatable := node.Status.Allocatable.Cpu().AsApproximateFloat64()

	for i, p := range nodePods {
		podMetrics := nodePodsMetricses[i]
		assumedPodCPUUsage := AssumedPodCPUUsage(p, assumedCPUUsageRate)
		realPodCPUUsage := PodCPUUsage(podMetrics)
		if realPodCPUUsage < assumedPodCPUUsage*lowerLimitCPUUsageRate {
			nodeCPUUsage += assumedPodCPUUsage - realPodCPUUsage
		}
	}

	before = nodeCPUUsage / nodeCPUAllocatable

	podCPURequest := PodCPURequest(pod)
	podCPULimit := PodCPULimit(pod)

	if podCPURequest != 0 {
		after = (nodeCPUUsage + podCPURequest) / nodeCPUAllocatable
	} else {
		after = (nodeCPUUsage + podCPULimit*assumedCPUUsageRate) / nodeCPUAllocatable
	}

	return
}

// AssumedPodCPUUsage assumes the total CPU usage by the given Pod.
//
// ContainerCPUUsage = requests.cpu || limits.cpu * assumedCPUUsageRate
// PodCPUUsage = [sum(ContainerCPUUsage(c)) for c in spec.containers]
// Ignore Pods with no requests.cpu and limits.cpu
func AssumedPodCPUUsage(pod *corev1.Pod, assumedCPUUsageRate float64) (v float64) {
	for _, c := range pod.Spec.Containers {
		vv := c.Resources.Requests.Cpu().AsApproximateFloat64()
		if vv == 0 {
			vv = c.Resources.Limits.Cpu().AsApproximateFloat64() * assumedCPUUsageRate
		}
		v += vv
	}
	return
}

func PodCPUUsage(podMetrics *metricsv1beta1.PodMetrics) (v float64) {
	for _, c := range podMetrics.Containers {
		v += c.Usage.Cpu().AsApproximateFloat64()
	}
	return
}

func PodCPURequest(pod *corev1.Pod) (v float64) {
	for _, c := range pod.Spec.Containers {
		v += c.Resources.Requests.Cpu().AsApproximateFloat64()
	}
	return
}

func PodCPULimit(pod *corev1.Pod) (v float64) {
	for _, c := range pod.Spec.Containers {
		v += c.Resources.Limits.Cpu().AsApproximateFloat64()
	}
	return
}
