package minimizepower

import (
	"context"
	"math"

	waometric "github.com/waok8s/wao-metrics-adapter/pkg/metric"
	"github.com/waok8s/wao-scheduler/pkg/predictor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetrics "k8s.io/metrics/pkg/client/custom_metrics"
)

const (
	// AssumedCPUUsageRate sets Pod CPU usage to (limits.cpu * AssumedCPUUsageRate) if requests.cpu is empty.
	AssumedCPUUsageRate = 0.85
	// LowerLimitCPUUsageRate sets the lowest Pod CPU usage to (limits.cpu * LowerLimitCPUUsageRate) for the score calculation.
	LowerLimitCPUUsageRate = 0.50
)

type MinimizePower struct {
	clientset            kubernetes.Interface
	metricsclientset     metricsv1beta1.MetricsV1beta1Interface
	custommetricsclient  custommetrics.CustomMetricsClient
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

	mc, err := metricsv1beta1.NewForConfig(cfg)
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
	avg := custommetrics.NewAvailableAPIsGetter(dc)
	cmc := custommetrics.NewForConfig(cfg, rm, avg)

	return &MinimizePower{
		clientset:            fh.ClientSet(),
		metricsclientset:     mc,
		custommetricsclient:  cmc,
		snapshotSharedLister: fh.SnapshotSharedLister(),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (*MinimizePower) Name() string {
	return Name
}

// ScoreExtensions returns a ScoreExtensions interface.
func (pl *MinimizePower) ScoreExtensions() framework.ScoreExtensions { return pl }

func (pl *MinimizePower) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	klog.InfoS("MinimizePower.Score", "node", nodeName, "pod", pod.Name)

	nodeInfo, err := pl.snapshotSharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=0 as error occurred", "node", nodeName, "pod", pod.Name)
		return 0, nil
	}
	node := nodeInfo.Node()
	nodeMetrics, err := pl.metricsclientset.NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		// TODO: fail
	}
	nodePods := make([]*corev1.Pod, len(nodeInfo.Pods))
	nodePodsMetricses := make([]*v1beta1.PodMetrics, len(nodeInfo.Pods))
	for i, p := range nodeInfo.Pods {
		podMetrics, err := pl.metricsclientset.PodMetricses(p.Pod.Namespace).Get(ctx, p.Pod.Name, metav1.GetOptions{})
		if err != nil {
			// TODO: skip the pod and log
		}
		nodePods[i] = p.Pod
		nodePodsMetricses[i] = podMetrics
	}

	beforeUsage, afterUsage := pl.CalcCPUUsage(ctx, node, nodeMetrics, nodePods, nodePodsMetricses, pod, AssumedCPUUsageRate, LowerLimitCPUUsageRate)
	if beforeUsage == afterUsage {
		// TODO: cannot fail so score 0?
	}
	if afterUsage > 1 {
		// TODO: cannot fail so score 0?
	}

	inletTemp, err := GetCustomMetricForNode(pl.custommetricsclient, nodeName, waometric.ValueInletTemperature)
	if err != nil {
		// TODO: score 0 ?
	}
	deltaP, err := GetCustomMetricForNode(pl.custommetricsclient, nodeName, waometric.ValueDeltaPressure)
	if err != nil {
		// TODO: score 0 ?
	}

	var pcp predictor.PowerConsumptionPredictor // TODO: init predictor
	beforeWatt, err := pcp.Predict(ctx, beforeUsage, inletTemp, deltaP)
	if err != nil {
		// TODO: score 0 ?
	}
	afterWatt, err := pcp.Predict(ctx, afterUsage, inletTemp, deltaP)
	if err != nil {
		// TODO: score 0 ?
	}
	score := int64(afterWatt - beforeWatt)
	return score, nil
}

func (pl *MinimizePower) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	klog.InfoS("MinimizePower.NormalizeScore", "pod", pod.Name)

	for node, score := range scores {
		if score.Score < 0 {
			// TODO: print warning
			scores[node].Score = 0
		}
	}

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

	klog.InfoS("MinimizePower.NormalizeScore", "pod", pod.Name, "scores", scores)

	return nil
}

// CalcCPUUsage calcs node CPU usage.
//
//   - Node CPU usage may be greater than 1.0.
//   - Containers are ignored when it has empty requests.cpu and empty limit.cpu.
//   - assumedCPUUsageRate is used when a container has empty requests.cpu and non-empty limits.cpu.
//   - lowerLimitCPUUsageRate is the lower limit CPU usage rate of a pod.
func (pl *MinimizePower) CalcCPUUsage(ctx context.Context,
	node *corev1.Node, nodeMetrics *v1beta1.NodeMetrics,
	nodePods []*corev1.Pod, nodePodsMetricses []*v1beta1.PodMetrics,
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

func PodCPUUsage(podMetrics *v1beta1.PodMetrics) (v float64) {
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

func GetCustomMetricForNode(client custommetrics.CustomMetricsClient, nodeName, metricName string) (float64, error) {
	mv, err := client.RootScopedMetrics().GetForObject(schema.GroupKind{Group: "", Kind: "node"}, nodeName, metricName, labels.NewSelector())
	if err != nil {
		return 0.0, err
	}
	return mv.Value.AsApproximateFloat64(), nil
}
