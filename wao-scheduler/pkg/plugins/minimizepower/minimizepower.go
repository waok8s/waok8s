package minimizepower

import (
	"context"
	"fmt"
	"math"
	"time"

	waometric "github.com/waok8s/wao-metrics-adapter/pkg/metric"
	waov1beta1 "github.com/waok8s/wao-nodeconfig/api/v1beta1"
	"github.com/waok8s/wao-scheduler/pkg/predictor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	snapshotSharedLister framework.SharedLister
	clientset            kubernetes.Interface
	ctrlclient           client.Client

	metricsclient   *CachedMetricsClient
	predictorclient *CachedPredictorClient
}

var _ framework.ScorePlugin = (*MinimizePower)(nil)
var _ framework.ScoreExtensions = (*MinimizePower)(nil)

var (
	Name = "MinimizePower"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(waov1beta1.AddToScheme(scheme))
}

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

	// init controller-runtime client
	ca, err := cache.New(fh.KubeConfig(), cache.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	go ca.Start(context.TODO())
	c, err := client.New(fh.KubeConfig(), client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: ca},
	})
	if err != nil {
		return nil, err
	}

	return &MinimizePower{
		snapshotSharedLister: fh.SnapshotSharedLister(),
		clientset:            fh.ClientSet(),
		ctrlclient:           c,
		metricsclient:        NewCachedMetricsClient(mc, cmc, MetricsCacheTTL),
		predictorclient:      NewCachedPredictorClient(c, PredictorCacheTTL),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (*MinimizePower) Name() string { return Name }

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
	nodeMetrics, err := pl.metricsclient.GetNodeMetrics(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// get pods and pods metrics, ignore pods that cannot get metrics
	var nodePods []*corev1.Pod
	var nodePodsMetricses []*metricsv1beta1.PodMetrics
	for i, p := range nodeInfo.Pods {
		podMetrics, err := pl.metricsclient.GetPodMetrics(ctx, p.Pod.Namespace, p.Pod.Name)
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
	inletTemp, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometric.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	deltaP, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometric.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := pl.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	for _, e := range ncs.Items {
		if e.Spec.NodeName == nodeName {
			nc = &e
			break
		}
	}
	if nc == nil {
		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}

	// init predictor endpoint
	var ep *waov1beta1.EndpointTerm
	if nc.Spec.Predictor.PowerConsumption != nil {
		ep = nc.Spec.Predictor.PowerConsumption
	} else {
		ep = &waov1beta1.EndpointTerm{}
	}

	if nc.Spec.Predictor.PowerConsumptionEndpointProvider != nil {
		ep2, err := pl.predictorclient.GetPredictorEndpoint(ctx, nc.Namespace, nc.Spec.Predictor.PowerConsumptionEndpointProvider, predictor.TypePowerConsumption)
		if err != nil {
			klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
			return math.MaxInt64, nil
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
		return math.MaxInt64, nil
	}
	afterWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, afterUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "node", nodeName, "pod", pod.Name)
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
