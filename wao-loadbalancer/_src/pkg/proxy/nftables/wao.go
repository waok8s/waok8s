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
)

var (
	scheme               = runtime.NewScheme()
	defaultCPUPerRequest = resource.MustParse(DefaultCPUPerRequest)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(waov1beta1.AddToScheme(scheme))
}

type WAOLB struct {
	ipFamily corev1.IPFamily

	// k8sclient       *kubernetes.Clientset
	ctrlclient      client.Client
	metricsclient   *waoclient.CachedMetricsClient
	predictorclient *waoclient.CachedPredictorClient

	// nodeNames     []string
	// endpoint2Node map[string]string
	// nodeScores    map[string]int64

	// scores is a map[svcPortNameString]map[endpointIP]score, initialized and used in the Proxier.
	scores map[string]map[string]int
}

func NewWAOLB(ipFamily corev1.IPFamily) (*WAOLB, error) {

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
		ipFamily: ipFamily,

		// k8sclient:       clientSet,
		ctrlclient:      c,
		metricsclient:   waoclient.NewCachedMetricsClient(mc, cmc, DefaultMetricsCacheTTL),
		predictorclient: waoclient.NewCachedPredictorClient(clientSet, DefaultMetricsCacheTTL),

		// nodeNames:     []string{},
		// endpoint2Node: make(map[string]string),
		// nodeScores:    make(map[string]int64),
	}, nil
}

// // GetNodesName lists ready nodes in the cluster.
// func (w *WAOLB) GetNodesName() {
// 	nodesName := []string{}
// 	nodes, err := w.k8sclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
// 	if err != nil {
// 		klog.Warningf("Cannot get list of nodes. Error : %v", err)
// 	}

// 	for _, node := range nodes.Items {
// 		for _, nodeStatus := range node.Status.Conditions {
// 			if nodeStatus.Type == corev1.NodeReady && nodeStatus.Status == corev1.ConditionTrue {
// 				nodesName = append(nodesName, node.Name)
// 			}
// 		}
// 	}
// 	w.nodeNames = nodesName
// }

// // GetPodsEndpoint lists all running pods and their endpoints.
// func (w *WAOLB) GetPodsEndpoint() {
// 	endpointsBelongNode := make(map[string]string)

// 	pods, err := w.k8sclient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
// 	if err != nil {
// 		klog.Warningf("Cannot get list of pods. Error : %v", err)
// 	}

// 	for _, pod := range pods.Items {
// 		if pod.Status.Phase == corev1.PodRunning {
// 			endpointsBelongNode[pod.Status.PodIP] = pod.Spec.NodeName
// 		}
// 	}
// 	w.endpoint2Node = endpointsBelongNode
// }

// func (w *WAOLB) CollectNodeAndPodList() {
// 	w.GetNodesName()
// 	w.GetPodsEndpoint()
// 	klog.Infof("NodesName: %#v", w.nodeNames)
// 	klog.Infof("Endpoints: %#v", w.endpoint2Node)
// }

// func (w *WAOLB) CalcNodesScore() {
// 	piece := len(w.nodeNames)
// 	workqueue.ParallelizeUntil(context.TODO(), Parallelism, piece, func(piece int) {
// 		w.nodeScores[w.nodeNames[piece]] = int64(w.Score(w.nodeNames[piece]))
// 	}, betterChunkSize(piece, Parallelism))
// 	klog.Infof("NodesScore: %#v", w.nodeScores)
// }

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

// // Score calculates node score.
// // The returned score is the amount of increase in current power consumption.
// func (w *WAOLB) Score(nodeName string) int64 {
// 	klog.V(5).Infof("%v : Start Score() function", nodeName)

// 	ctx := context.TODO()

// 	nodeInfo, err := w.k8sclient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
// 	if err != nil {
// 		klog.Errorf("%v : Cannot get Nodes info. Error : %v", nodeName, err)
// 		return -1
// 	}

// 	// get node metrics
// 	nodeMetrics, err := w.metricsclient.GetNodeMetrics(ctx, nodeName)
// 	if err != nil {
// 		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}

// 	// prepare beforeUsage and afterUsage
// 	beforeUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()

// 	//
// 	nodeResource := nodeInfo.Status.Capacity["cpu"]
// 	nodeCPUCapacity, _ := strconv.ParseFloat(nodeResource.AsDec().String(), 32)
// 	cpuCapacity := float64(nodeCPUCapacity)
// 	klog.InfoS("wao.Score usage", "node", nodeName, "usage_before", beforeUsage)

// 	beforeUsage = (beforeUsage / cpuCapacity) * 100
// 	klog.InfoS("wao.Score usage (formatted)", "node", nodeName, "usage_before", beforeUsage, "cpu_capacity", cpuCapacity)

// 	// get custom metrics
// 	inletTemp, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
// 	if err != nil {
// 		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}
// 	deltaP, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
// 	if err != nil {
// 		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}
// 	klog.InfoS("wao.Score metrics", "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

// 	// get NodeConfig
// 	var nc *waov1beta1.NodeConfig
// 	var ncs waov1beta1.NodeConfigList
// 	if err := w.ctrlclient.List(ctx, &ncs); err != nil {
// 		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}
// 	for _, e := range ncs.Items {
// 		klog.Infof("e: %v", e)
// 		// TODO: handle node with multiple NodeConfig
// 		if e.Spec.NodeName == nodeName {
// 			nc = e.DeepCopy()
// 			break
// 		}
// 	}
// 	if nc == nil {
// 		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}

// 	// init predictor endpoint
// 	var ep *waov1beta1.EndpointTerm
// 	if nc.Spec.Predictor.PowerConsumption != nil {
// 		ep = nc.Spec.Predictor.PowerConsumption
// 	} else {
// 		ep = &waov1beta1.EndpointTerm{}
// 	}

// 	if nc.Spec.Predictor.PowerConsumptionEndpointProvider != nil {
// 		ep2, err := w.predictorclient.GetPredictorEndpoint(ctx, nc.Namespace, nc.Spec.Predictor.PowerConsumptionEndpointProvider, predictor.TypePowerConsumption)
// 		if err != nil {
// 			klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 			return -1
// 		}
// 		ep.Type = ep2.Type
// 		ep.Endpoint = ep2.Endpoint
// 	}

// 	// do predict
// 	beforeWatt, err := w.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
// 	if err != nil {
// 		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
// 		return -1
// 	}
// 	klog.InfoS("wao.Score prediction", "node", nodeName, "watt_before", beforeWatt)

// 	return int64(beforeWatt)
// }

// func (w *WAOLB) CalcModRanges(endpointList []string) (modRanges []int64) {
// 	if len(endpointList) == 0 {
// 		return
// 	}

// 	minScore := int64(math.MaxInt64)
// 	for _, ip := range endpointList {
// 		score, ok := w.nodeScores[w.endpoint2Node[ip]]
// 		if !ok || score <= 0 {
// 			continue
// 		}
// 		if score < minScore {
// 			minScore = score
// 		}
// 	}
// 	if minScore == int64(math.MaxInt64) {
// 		return
// 	}

// 	for _, ip := range endpointList {
// 		score, ok := w.nodeScores[w.endpoint2Node[ip]]
// 		modRange := int64(0)
// 		if ok && score > 0 {
// 			modRange = int64(MaxModRange * minScore / score)
// 		}
// 		modRanges = append(modRanges, modRange)
// 	}
// 	return
// }

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
		if e.AddressType == discoveryv1.AddressType(w.ipFamily) {
			es = e.DeepCopy()
			break
		}
	}
	if es == nil {
		err := fmt.Errorf("EndpointSlice not found svc=%s ipFamily=%s", svc.Name, w.ipFamily)
		klog.ErrorS(err, "WAO: ScoreService failed to get EndpointSlice with same IPFamily", "svc", types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, "ipFamily", w.ipFamily)
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
func (w *WAOLB) ScoreNode(ctx context.Context, nodeName string, cpuUsage resource.Quantity) (int, error) {
	klog.V(5).InfoS("WAO: ScoreNode", "nodeName", nodeName, "cpuUsage", cpuUsage)

	panic("not implemented")
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
