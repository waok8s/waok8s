package minimizepower

import (
	"context"
	"fmt"
	"math"

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

	waov1beta1 "github.com/waok8s/waok8s/wao-core/api/wao/v1beta1"
	waoclient "github.com/waok8s/waok8s/wao-core/pkg/client"
	waometrics "github.com/waok8s/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/waok8s/wao-core/pkg/predictor"
)

type MinimizePower struct {
	snapshotSharedLister framework.SharedLister
	ctrlclient           client.Client

	metricsclient   *waoclient.CachedMetricsClient
	predictorclient *waoclient.CachedPredictorClient

	args *MinimizePowerArgs
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

func getArgs(obj runtime.Object) (MinimizePowerArgs, error) {
	ptr, ok := obj.(*MinimizePowerArgs)
	if !ok {
		return MinimizePowerArgs{}, fmt.Errorf("want args to be of type MinimizePowerArgs, got %T", obj)
	}
	ptr.Default()
	if err := ptr.Validate(); err != nil {
		return MinimizePowerArgs{}, err
	}
	return *ptr, nil
}

// New initializes a new plugin and returns it.
func New(_ context.Context, obj runtime.Object, fh framework.Handle) (framework.Plugin, error) {

	// get plugin args
	args, err := getArgs(obj)
	if err != nil {
		return nil, err
	}
	klog.InfoS("MinimizePower.New", "args", args)

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
	ca, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	go ca.Start(context.TODO()) // NOTE: this context needs live until the scheduler stops
	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: ca},
	})
	if err != nil {
		return nil, err
	}

	return &MinimizePower{
		snapshotSharedLister: fh.SnapshotSharedLister(),
		ctrlclient:           c,
		metricsclient:        waoclient.NewCachedMetricsClient(mc, cmc, args.MetricsCacheTTL.Duration),
		predictorclient:      waoclient.NewCachedPredictorClient(fh.ClientSet(), args.PredictorCacheTTL.Duration),
		args:                 &args,
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

var (
	// ScoreBase is the base score for all nodes.
	// This is the lowest score except for special scores (will be replaced to 0,1,...,<ScoreBase)
	ScoreBase int64 = 20
)

const (
	ScoreError int64 = math.MaxInt64
	ScoreMax   int64 = math.MaxInt64 >> 1
)

var (
	// ScoreReplaceMap are scores that have special meanings.
	// NormalizeScore will replace them with the mapped values (should be less than ScoreBase).
	ScoreReplaceMap = map[int64]int64{
		ScoreError: 0,
		ScoreMax:   1,
	}
)

// Score returns how many watts will be increased by the given pod (lower is better).
//
// This function never returns an error (as errors cause the pod to be rejected).
// If an error occurs, it is logged and the score is set to ScoreError(math.MaxInt64).
func (pl *MinimizePower) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	klog.InfoS("MinimizePower.Score", "pod", pod.Name, "node", nodeName)

	nodeInfo, err := pl.snapshotSharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}

	// get node and node metrics
	node := nodeInfo.Node()
	nodeMetrics, err := pl.metricsclient.GetNodeMetrics(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}

	// get additional usage (add assumed pod CPU usage for not running pods)
	// NOTE: We need to assume the CPU usage of pods that have scheduled to this node but are not yet running,
	// as the next replica (if the pod belongs to a deployment, etc.) will be scheduled soon and we need to consider the CPU usage of these pods.
	var assumedAdditionalUsage float64
	for _, p := range nodeInfo.Pods {
		if p.Pod.Spec.NodeName != node.Name {
			continue
		}
		if p.Pod.Status.Phase == corev1.PodRunning ||
			p.Pod.Status.Phase == corev1.PodSucceeded ||
			p.Pod.Status.Phase == corev1.PodFailed ||
			p.Pod.Status.Phase == corev1.PodUnknown {
			continue // only pending pods are counted
		}
		// NOTE: No need to check pod.Status.Conditions as pods on this node with pending status are just what we want.
		// However, pods that have just been started and are not yet using CPU are not counted. (this is a restriction for now)
		assumedAdditionalUsage += PodCPURequestOrLimit(p.Pod) * pl.args.PodUsageAssumption
	}
	// prepare beforeUsage and afterUsage
	beforeUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()
	beforeUsage += assumedAdditionalUsage
	afterUsage := beforeUsage + PodCPURequestOrLimit(pod)
	if beforeUsage == afterUsage { // The Pod has both requests.cpu and limits.cpu empty or zero. Normally, this should not happen.
		klog.ErrorS(fmt.Errorf("beforeUsage == afterUsage v=%v", beforeUsage), "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	// NOTE: Normally, status.capacity.cpu and status.allocatable.cpu are the same.
	cpuCapacity := node.Status.Capacity.Cpu().AsApproximateFloat64()
	if afterUsage > cpuCapacity { // CPU overcommitment, make the node nearly lowest priority.
		klog.InfoS("MinimizePower.Score score=ScoreMax as CPU overcommitment", "pod", pod.Name, "node", nodeName, "usage_after", afterUsage, "cpu_capacity", cpuCapacity)
		return ScoreMax, nil
	}
	klog.InfoS("MinimizePower.Score usage", "pod", pod.Name, "node", nodeName, "usage_before", beforeUsage, "usage_after", afterUsage, "additional_usage_included", assumedAdditionalUsage)

	// format usage
	switch pl.args.CPUUsageFormat {
	case CPUUsageFormatRaw:
		// do nothing
	case CPUUsageFormatPercent:
		beforeUsage = (beforeUsage / cpuCapacity) * 100
		afterUsage = (afterUsage / cpuCapacity) * 100
	default:
		// this never happens as args.Validate() checks the value
	}
	klog.InfoS("MinimizePower.Score usage (formatted)", "pod", pod.Name, "node", nodeName, "format", pl.args.CPUUsageFormat, "usage_before", beforeUsage, "usage_after", afterUsage, "cpu_capacity", cpuCapacity)

	// get custom metrics
	inletTemp, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	deltaP, err := pl.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	klog.InfoS("MinimizePower.Score metrics", "pod", pod.Name, "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := pl.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	for _, e := range ncs.Items {
		// TODO: handle node with multiple NodeConfig
		if e.Spec.NodeName == nodeName {
			nc = e.DeepCopy()
			break
		}
	}
	if nc == nil {
		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
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
			klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
			return ScoreError, nil
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	afterWatt, err := pl.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, afterUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "MinimizePower.Score score=ScoreError as error occurred", "pod", pod.Name, "node", nodeName)
		return ScoreError, nil
	}
	klog.InfoS("MinimizePower.Score prediction", "pod", pod.Name, "node", nodeName, "watt_before", beforeWatt, "watt_after", afterWatt)

	podPowerConsumption := int64(afterWatt - beforeWatt)
	if podPowerConsumption < 0 {
		klog.InfoS("MinimizePower.Score round negative scores to 0", "pod", pod.Name, "node", nodeName, "watt", afterWatt-beforeWatt)
		podPowerConsumption = 0
	}

	return podPowerConsumption, nil
}

func (pl *MinimizePower) NormalizeScore(_ context.Context, _ *framework.CycleState, pod *corev1.Pod, scores framework.NodeScoreList) *framework.Status {
	klog.InfoS("MinimizePower.NormalizeScore before", "pod", pod.Name, "scores", scores)

	PowerConsumptions2Scores(scores, ScoreBase, ScoreReplaceMap)

	klog.InfoS("MinimizePower.NormalizeScore after", "pod", pod.Name, "scores", scores)

	return nil
}

func PowerConsumptions2Scores(scores framework.NodeScoreList, baseScore int64, replaceMap map[int64]int64) {

	var replacedScores []framework.NodeScore
	var calculatedScores []framework.NodeScore

	for _, score := range scores {
		if newScore, ok := replaceMap[score.Score]; ok {
			replacedScores = append(replacedScores, framework.NodeScore{Name: score.Name, Score: newScore})
		} else {
			calculatedScores = append(calculatedScores, framework.NodeScore{Name: score.Name, Score: score.Score})
		}
	}

	// normalize calculatedScores
	highest := int64(math.MinInt64)
	lowest := int64(math.MaxInt64)
	for _, score := range calculatedScores {
		if score.Score > highest {
			highest = score.Score
		}
		if score.Score < lowest {
			lowest = score.Score
		}
	}
	for node, score := range calculatedScores {
		if highest != lowest {
			maxNodeScore := int64(framework.MaxNodeScore)
			minNodeScore := int64(baseScore)
			calculatedScores[node].Score = int64(maxNodeScore - ((maxNodeScore - minNodeScore) * (score.Score - lowest) / (highest - lowest)))
		} else {
			calculatedScores[node].Score = baseScore
		}
	}

	// concat replacedScores and calculatedScores
	scores2 := map[string]framework.NodeScore{}
	for _, score := range replacedScores {
		scores2[score.Name] = score
	}
	for _, score := range calculatedScores {
		scores2[score.Name] = score
	}

	// replace scores
	for i, score := range scores {
		scores[i] = scores2[score.Name]
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
