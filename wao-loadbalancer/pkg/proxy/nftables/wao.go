package nftables

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	// scheme
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	// kubeconfig
	"k8s.io/client-go/rest"

	// controller-runtime
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// metrics
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"

	// wao
	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	waoclient "github.com/waok8s/wao-core/pkg/client"
	waometrics "github.com/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/wao-core/pkg/predictor"
)

// Copied from wao-scheduler.
const (
	CPUUsageFormatRaw     string = "Raw"
	CPUUsageFormatPercent string = "Percent"
)

const (
	// NFTableNameWAONode is the name of the nftables table for WAO Load Balancer.
	NFTableNameWAOLB = "wao-loadbalancer"

	AnnotationCPUPerRequest = "waok8s.github.io/cpu-per-request"

	DefaultCPUPerRequest = "100m"

	// Parallelism is the number of goroutines to use for parallelizing work.
	// See: kubernetes/pkg/scheduler/framework/parallelize
	Parallelism = 64

	// MaxModRange = int64(100)
	ScoreMax = 100
	ScoreMin = 0

	DefaultMetricsCacheTTL   = 30 * time.Second
	DefaultPredictorCacheTTL = 30 * time.Minute

	DefaultCPUUsageFormat = CPUUsageFormatPercent
)

var (
	scheme               = runtime.NewScheme()
	defaultCPUPerRequest = resource.MustParse(DefaultCPUPerRequest)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(waov1beta1.AddToScheme(scheme))
}

type WAOLBOptions struct {
	IPFamily corev1.IPFamily

	MetricsCacheTTL   time.Duration
	PredictorCacheTTL time.Duration

	CPUUsageFormat string
}

func DefaultingWAOLBOptions(opts WAOLBOptions) WAOLBOptions {
	if opts.IPFamily == "" {
		// This is required; no default value.
		// NOTE: "" is interpreted as "corev1.IPFamilyUnknown" in the k8s codebase.
	}
	if opts.MetricsCacheTTL == 0 {
		opts.MetricsCacheTTL = DefaultMetricsCacheTTL
	}
	if opts.PredictorCacheTTL == 0 {
		opts.PredictorCacheTTL = DefaultPredictorCacheTTL
	}
	if opts.CPUUsageFormat == "" {
		opts.CPUUsageFormat = DefaultCPUUsageFormat
	}
	return opts
}

func ValidatingWAOLBOptions(opts WAOLBOptions) error {
	if opts.IPFamily == "" {
		return fmt.Errorf("ValidatingWAOLBOptions: IPFamily is required")
	}
	if opts.MetricsCacheTTL <= 0 {
		return fmt.Errorf("ValidatingWAOLBOptions: MetricsCacheTTL must be positive")
	}
	if opts.PredictorCacheTTL <= 0 {
		return fmt.Errorf("ValidatingWAOLBOptions: PredictorCacheTTL must be positive")
	}
	if opts.CPUUsageFormat != CPUUsageFormatRaw && opts.CPUUsageFormat != CPUUsageFormatPercent {
		return fmt.Errorf("ValidatingWAOLBOptions: CPUUsageFormat must be either `Raw` or `Percent`")
	}
	return nil
}

type WAOLB struct {
	opts WAOLBOptions

	ctrlclient      client.Client
	metricsclient   *waoclient.CachedMetricsClient
	predictorclient *waoclient.CachedPredictorClient

	// Scores is a map[svcPortNameString]map[endpointIP]score.
	// Score() calculates the scores and updates this map.
	// To read the scores, users should carefully use this map directly.
	// This map is not thread-safe.
	Scores map[string]map[string]int
}

func NewWAOLB(opts WAOLBOptions) (*WAOLB, error) {

	opts = DefaultingWAOLBOptions(opts)
	if err := ValidatingWAOLBOptions(opts); err != nil {
		return nil, err
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// init kubernetes client
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

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
	go ca.Start(context.TODO()) // NOTE: this context needs live until the kube-proxy stops
	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: ca},
	})
	if err != nil {
		return nil, err
	}

	return &WAOLB{
		opts: opts,

		ctrlclient:      c,
		metricsclient:   waoclient.NewCachedMetricsClient(mc, cmc, opts.MetricsCacheTTL),
		predictorclient: waoclient.NewCachedPredictorClient(clientSet, opts.PredictorCacheTTL),
	}, nil
}

// betterChunkSize is a helper function to calculate the chunk size for parallel work.
// It returns max(1, min(sqrt(n), n/Parallelism)) in workqueue.Options format.
// See: kubernetes/pkg/scheduler/framework/parallelize
func betterChunkSize(n, parallelism int) workqueue.Options {
	s := int(math.Sqrt(float64(n)))
	if r := n/parallelism + 1; s > r {
		s = r
	} else if s < 1 {
		s = 1
	}
	return workqueue.WithChunkSize(s)
}

// Score concurrently calculates scores of all services and updates w.scores.
// It clears w.scores before calculating scores.
// If an error occurs during the calculation of a service, the service is ignored.
// See ScoreService and ScoreNode for details.
func (w *WAOLB) Score(ctx context.Context, svcPortNames []string) {
	klog.V(5).InfoS("WAO: Score", "len(svcPortNames)", len(svcPortNames))

	w.Scores = map[string]map[string]int{} // map[svcPortNameString]map[endpointIP]score
	klog.V(5).InfoS("WAO: Score cleared w.scores")

	// NOTE: w.Scores is not thread-safe, but we don't update the same key in parallel, so it's safe here.
	klog.InfoS("WAO: Score start parallelizing", "n", len(svcPortNames), "parallelism", Parallelism)
	n := len(svcPortNames)
	workqueue.ParallelizeUntil(ctx, Parallelism, n, func(piece int) {
		svcPortName := svcPortNames[piece]
		svcNS, svcName, _ := decodeSvcPortNameString(svcPortName)
		scores, err := w.ScoreService(ctx, types.NamespacedName{Namespace: svcNS, Name: svcName})
		if err != nil {
			klog.ErrorS(err, "WAO: Score failed to score service", "svcPortName", svcPortName)
			return
		}
		w.Scores[svcPortName] = scores // set the value only if no error
		klog.V(5).InfoS("WAO: Score added scores", "svcPortName", svcPortName)
	}, betterChunkSize(n, Parallelism))

	klog.V(5).InfoS("WAO: Score updated w.scores", "len(svcPortNames)", len(svcPortNames), "len(w.scores)", len(w.Scores))
}

// ScoreService calculates scores of the given Service for all nodes.
// Returns map[endpointIP]score. The score is in [0, 100].
func (w *WAOLB) ScoreService(ctx context.Context, svcName types.NamespacedName) (map[string]int, error) {
	klog.V(5).InfoS("WAO: ScoreService", "svcName", svcName)

	// get service
	var svc corev1.Service
	if err := w.ctrlclient.Get(ctx, svcName, &svc); err != nil {
		klog.ErrorS(err, "WAO: ScoreService failed to get Service", "svcName", svcName)
		return nil, err
	}
	klog.V(5).InfoS("WAO: ScoreService Service", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name})

	// get cpu-per-request
	cpuPerRequest, err := resource.ParseQuantity(svc.Annotations[AnnotationCPUPerRequest])
	if err != nil {
		cpuPerRequest = defaultCPUPerRequest
		klog.V(5).InfoS("WAO: ScoreService using default cpu-per-request (annotation not found or parsing error)", "svc", svc.Name, "annotation", AnnotationCPUPerRequest)
	}
	klog.V(5).InfoS("WAO: ScoreService CPUPerRequest", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "cpuPerRequest", cpuPerRequest.String())

	// get endpointSlice
	var es *discoveryv1.EndpointSlice
	var ess discoveryv1.EndpointSliceList
	if err := w.ctrlclient.List(ctx, &ess, client.InNamespace(svc.Namespace), client.MatchingLabels{"kubernetes.io/service-name": svc.Name}); err != nil {
		klog.ErrorS(err, "WAO: ScoreService failed to list EndpointSlice", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name})
		return nil, err
	}
	for _, e := range ess.Items {
		if e.AddressType == discoveryv1.AddressType(w.opts.IPFamily) {
			es = e.DeepCopy()
			break
		}
	}
	if es == nil {
		err := fmt.Errorf("EndpointSlice not found svc=%s ipFamily=%s", svc.Name, w.opts.IPFamily)
		klog.ErrorS(err, "WAO: ScoreService failed to get EndpointSlice with same IPFamily", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "ipFamily", w.opts.IPFamily)
		return nil, err
	}
	klog.V(5).InfoS("WAO: ScoreService EndpointSlice", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "endpointSlice", types.NamespacedName{Namespace: es.Namespace, Name: es.Name})

	// get scores (in watt)
	watts := map[string]int{} // map[endpointIP]watt
	for _, ep := range es.Endpoints {
		// NOTE: we don't check conditions (ready, serving, terminating) of the endpoint here,
		// because the proxier which calls this function knows which endpoints are ready.
		// So, we just calculate scores for all endpoints in the EndpointSlice.
		var nodeName string
		if ep.NodeName != nil {
			nodeName = *ep.NodeName
		}
		watt, err := w.ScoreNode(ctx, nodeName, cpuPerRequest)
		if err != nil {
			klog.ErrorS(err, "WAO: ScoreService ScoreNode failed, so ignore this endpoint", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "endpointSlice", types.NamespacedName{Namespace: es.Namespace, Name: es.Name}, "endpointSlice.endpoints", ep)
		} else {
			// NOTE: if multiple addresses are assigned to the same NodeName, the same watt is assigned to all addresses
			for _, addr := range ep.Addresses {
				watts[addr] = watt
			}
		}
	}
	klog.V(5).InfoS("WAO: ScoreService watts", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "watts", watts)

	// normalize scores
	scores := normalizeScores(watts)
	klog.V(5).InfoS("WAO: ScoreService scores", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "scores", scores)

	// return
	return scores, nil
}

// ScoreNode returns the predicted delta power consumption of the given node. The returned value is in watt, not normalized.
// This logic is basically the same as the wao-scheduler code.
func (w *WAOLB) ScoreNode(ctx context.Context, nodeName string, cpuUsage resource.Quantity) (int, error) {
	klog.V(5).InfoS("WAO: ScoreNode", "nodeName", nodeName, "cpuUsage", cpuUsage.String())

	// get node and node metrics
	var node corev1.Node
	if err := w.ctrlclient.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		klog.ErrorS(err, "WAO: ScoreNode failed to get Node", "nodeName", nodeName)
		return 0, err
	}
	nodeMetrics, err := w.metricsclient.GetNodeMetrics(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "WAO: ScoreNode failed to get NodeMetrics", "nodeName", nodeName)
		return 0, err
	}

	// NOTE: WAO Load Balancer doesn't use assumedAdditionalUsage; so it's always 0.
	var assumedAdditionalUsage float64
	// prepare beforeUsage and afterUsage
	beforeUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()
	beforeUsage += assumedAdditionalUsage
	virtualPod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "WAOLB_VIRTUAL_POD",
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: cpuUsage,
						},
					},
				},
			},
		},
	}
	afterUsage := beforeUsage + PodCPURequestOrLimit(virtualPod)
	if beforeUsage == afterUsage { // The Pod has both requests.cpu and limits.cpu empty or zero. Normally, this should not happen.
		klog.ErrorS(fmt.Errorf("beforeUsage == afterUsage v=%v", beforeUsage), "WAO: ScoreNode error", "node", nodeName, "cpuUsage", cpuUsage.String())
		return 0, nil
	}
	// NOTE: Normally, status.capacity.cpu and status.allocatable.cpu are the same.
	cpuCapacity := node.Status.Capacity.Cpu().AsApproximateFloat64()
	if afterUsage > cpuCapacity { // CPU overcommitment
		// do nothing as this is a normal situation
	}
	klog.V(5).InfoS("WAO: ScoreNode usage", "node", nodeName, "cpuUsage", cpuUsage.String(), "usage_before", beforeUsage, "usage_after", afterUsage, "additional_usage_included", assumedAdditionalUsage)

	// format usage
	switch w.opts.CPUUsageFormat {
	case CPUUsageFormatRaw:
		// do nothing
	case CPUUsageFormatPercent:
		beforeUsage = (beforeUsage / cpuCapacity) * 100
		afterUsage = (afterUsage / cpuCapacity) * 100
	default:
		// this never happens as ValidatingWAOLBOptions() checks the value
	}
	klog.V(5).InfoS("WAO: ScoreNode usage (formatted)", "node", nodeName, "cpuUsage", cpuUsage.String(), "format", w.opts.CPUUsageFormat, "usage_before", beforeUsage, "usage_after", afterUsage, "cpu_capacity", cpuCapacity)

	// get custom metrics
	inletTemp, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "WAO: ScoreNode GetCustomMetricForNode", "node", nodeName, "metric", waometrics.ValueInletTemperature)
		return 0, err
	}
	deltaP, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "WAO: ScoreNode GetCustomMetricForNode", "node", nodeName, "metric", waometrics.ValueDeltaPressure)
		return 0, err
	}
	klog.V(5).InfoS("WAO: ScoreNode metrics", "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := w.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "WAO: ScoreNode failed to list NodeConfig", "node", nodeName)
		return 0, err
	}
	for _, e := range ncs.Items {
		// TODO: handle node with multiple NodeConfig
		if e.Spec.NodeName == nodeName {
			nc = e.DeepCopy()
			break
		}
	}
	if nc == nil {
		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "WAO: ScoreNode error", "node", nodeName)
		return 0, nil
	}

	// init predictor endpoint
	var ep *waov1beta1.EndpointTerm
	if nc.Spec.Predictor.PowerConsumption != nil {
		ep = nc.Spec.Predictor.PowerConsumption
	} else {
		ep = &waov1beta1.EndpointTerm{}
	}
	if nc.Spec.Predictor.PowerConsumptionEndpointProvider != nil {
		ep2, err := w.predictorclient.GetPredictorEndpoint(ctx, nc.Namespace, nc.Spec.Predictor.PowerConsumptionEndpointProvider, predictor.TypePowerConsumption)
		if err != nil {
			klog.ErrorS(err, "WAO: ScoreNode GetPredictorEndpoint", "node", nodeName)
			return 0, err
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := w.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "WAO: ScoreNode failed to predict power consumption", "node", nodeName)
		return 0, err
	}
	afterWatt, err := w.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, afterUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "WAO: ScoreNode failed to predict power consumption", "node", nodeName)
		return 0, err
	}
	klog.V(5).InfoS("WAO: ScoreNode prediction", "node", nodeName, "watt_before", beforeWatt, "watt_after", afterWatt)

	powerConsumption := int(afterWatt - beforeWatt)
	if powerConsumption < 0 {
		klog.InfoS("WAO: ScoreNode round negative scores to 0", "node", nodeName, "watt", afterWatt-beforeWatt)
		powerConsumption = 0
	}

	return powerConsumption, nil
}

// normalizeScores normalizes watts to score in [0, 100].
// The higher score means the lower power consumption (the order is reversed).
// The score is calculated by the formula: score_i = 100 * (min(watts) / watts_i).
// The returned map is map[endpointIP]score.
// Negative watt values are ignored. To avoid 0 watt, all watt values are increased by 1.
func normalizeScores(watts map[string]int) map[string]int {

	watts2 := map[string]int{}
	for ip, watt := range watts {
		if watt < 0 {
			continue
		}
		watts2[ip] = watt + 1
	}

	minWatt := math.MaxInt64
	for _, watt := range watts2 {
		if watt < minWatt {
			minWatt = watt
		}
	}

	scores := map[string]int{} // map[endpointIP]score
	for ip, watt := range watts2 {
		score := int(math.Round(float64(ScoreMax) * (float64(minWatt) / float64(watt))))
		score = max(ScoreMin, min(ScoreMax, score)) // this is just for safety
		scores[ip] = score
	}

	return scores
}

// decodeSvcPortNameString decodes svcPortNameString into namespace, svcName, and portName.
//
// "default/nginx" -> namespace="default", svcName="nginx", portName=""
// "default/nginx:http" -> namespace="default", svcName="nginx", portName="http"
func decodeSvcPortNameString(svcPortNameString string) (namespace string, svcName string, portName string) {
	v := strings.Split(svcPortNameString, "/")
	if len(v) != 2 {
		return
	}
	namespace = v[0]
	vv := strings.Split(v[1], ":")
	svcName = vv[0]
	if len(vv) == 2 {
		portName = vv[1]
	}
	return
}

// PodCPURequestOrLimit returns the sum of CPU requests or limits of the given pod.
// Copied from wao-scheduler.
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
