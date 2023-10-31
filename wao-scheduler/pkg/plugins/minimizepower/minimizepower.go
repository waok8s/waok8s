package minimizepower

import (
	"context"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	waoclient "github.com/waok8s/wao-core/pkg/client"
	waometrics "github.com/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/wao-core/pkg/predictor"
)

const (
	// MetricsCacheTTL is the expiration time of the metrics cache.
	MetricsCacheTTL = 15 * time.Second
	// PredictorCacheTTL is the expiration time of the predictor cache.
	PredictorCacheTTL = 10 * time.Minute
)

type MinimizePower struct {
	snapshotSharedLister framework.SharedLister
	ctrlclient           client.Client

	metricsclient   *waoclient.CachedMetricsClient
	predictorclient *waoclient.CachedPredictorClient
}

var _ framework.PreFilterPlugin = (*MinimizePower)(nil)
var _ framework.ScorePlugin = (*MinimizePower)(nil)
var _ framework.ScoreExtensions = (*MinimizePower)(nil)

var (
	Name = "MinimizePower"

	ReasonResourceRequest = "at least one container in the pod must have a requests.cpu or limits.cpu set"
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
		ctrlclient:           c,
		metricsclient:        waoclient.NewCachedMetricsClient(mc, cmc, MetricsCacheTTL),
		predictorclient:      waoclient.NewCachedPredictorClient(fh.ClientSet(), PredictorCacheTTL),
	}, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (*MinimizePower) Name() string { return Name }

// PreFilterExtensions returns nil as this plugin does not have PreFilterExtensions.
func (pl *MinimizePower) PreFilterExtensions() framework.PreFilterExtensions { return nil }

// PreFilter rejects a pod if it does not have at least one container that has a CPU request or limit set.
func (pl *MinimizePower) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	klog.InfoS("MinimizePower.PreFilter", "pod", pod.Name)

	if PodCPURequestOrLimit(pod) == 0 {
		return nil, framework.NewStatus(framework.Unschedulable, ReasonResourceRequest)
	}

	return &framework.PreFilterResult{NodeNames: nil}, nil
}

// ScoreExtensions returns a ScoreExtensions interface.
func (pl *MinimizePower) ScoreExtensions() framework.ScoreExtensions { return pl }

// Score returns how many watts will be increased by the given pod (lower is better).
//
// This function never returns an error (as errors cause the pod to be rejected).
// If an error occurs, it is logged and the score is set to math.MaxInt64.
func (pl *MinimizePower) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	klog.InfoS("MinimizePower.Score", "pod", pod.Name, "node", nodeName)

	nodeInfo, err := pl.snapshotSharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}

	// get node and node metrics
	node := nodeInfo.Node()
	nodeMetrics, err := pl.metricsclient.GetNodeMetrics(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}

	// prepare beforeUsage and afterUsage
	beforeUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()
	afterUsage := beforeUsage + PodCPURequestOrLimit(pod)
	if beforeUsage == afterUsage { // The Pod has both requests.cpu and limits.cpu empty or zero. Normally, this should not happen.
		klog.ErrorS(fmt.Errorf("beforeUsage == afterUsage v=%v", beforeUsage), "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}
	if afterUsage > 1 { // CPU overcommitment, make the node nearly lowest priority.
		klog.InfoS("MinimizePower.Score score=MaxInt64>>1 as CPU overcommitment", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64 >> 1, nil
	}

	// get custom metrics
	inletTemp, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}
	deltaP, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}

	klog.InfoS("MinimizePower.Score metrics", "pod", pod.Name, "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := pl.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}
	for _, e := range ncs.Items {
		// TODO: handle node with multiple NodeConfig
		if e.Spec.NodeName == nodeName {
			nc = &e
			break
		}
	}
	if nc == nil {
		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
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
			klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
			return math.MaxInt64, nil
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}
	afterWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, afterUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=MaxInt64 as error occurred", "pod", pod.Name, "node", nodeName)
		return math.MaxInt64, nil
	}
	klog.InfoS("MinimizePower.Score predicted", "pod", pod.Name, "node", nodeName, "watt_before", beforeWatt, "watt_after", afterWatt)

	podPowerConsumption := int64(afterWatt - beforeWatt)
	if podPowerConsumption < 0 {
		klog.InfoS("MinimizePower.Score round podPowerConsumption to 0", "pod", pod.Name, "node", nodeName, "podPowerConsumption", podPowerConsumption)
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

func PodCPURequestOrLimit(pod *corev1.Pod) (v float64) {
	for _, c := range pod.Spec.Containers {
		vv := c.Resources.Requests.Cpu().AsApproximateFloat64()
		if vv == 0 {
			vv = c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
		v += vv
	}
	return
}
