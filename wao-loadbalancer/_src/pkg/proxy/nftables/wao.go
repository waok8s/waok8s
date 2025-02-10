package nftables

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	custommetricsclient "k8s.io/metrics/pkg/client/custom_metrics"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	waoclient "github.com/waok8s/wao-core/pkg/client"
	waometrics "github.com/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/wao-core/pkg/predictor"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NFTableNameWAOLB = "wao-loadbalancer"
)

const (
	// Parallelism is the number of goroutines to use for parallelizing work.
	// See: kubernetes/pkg/scheduler/framework/parallelize
	Parallelism = 64

	MaxModRange = int64(100)

	DefaultMetricsCacheTTL   = 30 * time.Second
	DefaultPredictorCacheTTL = 30 * time.Minute
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(waov1beta1.AddToScheme(scheme))
}

type WAOLB struct {
	k8sclient       *kubernetes.Clientset
	ctrlclient      client.Client
	metricsclient   *waoclient.CachedMetricsClient
	predictorclient *waoclient.CachedPredictorClient

	nodeNames     []string
	endpoint2Node map[string]string
	nodeScores    map[string]int64
}

func NewWAOLB() (*WAOLB, error) {

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
		k8sclient:       clientSet,
		ctrlclient:      c,
		metricsclient:   waoclient.NewCachedMetricsClient(mc, cmc, DefaultMetricsCacheTTL),
		predictorclient: waoclient.NewCachedPredictorClient(clientSet, DefaultMetricsCacheTTL),
		nodeNames:       []string{},
		endpoint2Node:   make(map[string]string),
		nodeScores:      make(map[string]int64),
	}, nil
}

// Get list of nodes Name inside Cluster
func (w *WAOLB) GetNodesName() {
	nodesName := []string{}
	nodes, err := w.k8sclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Warningf("Cannot get list of nodes. Error : %v", err)
	}

	for _, node := range nodes.Items {
		for _, nodeStatus := range node.Status.Conditions {
			if nodeStatus.Type == v1.NodeReady && nodeStatus.Status == v1.ConditionTrue {
				nodesName = append(nodesName, node.Name)
			}
		}
	}
	w.nodeNames = nodesName
}

// Get list of pods endpoint inside Cluster
func (w *WAOLB) GetPodsEndpoint() {
	endpointsBelongNode := make(map[string]string)

	pods, err := w.k8sclient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Warningf("Cannot get list of pods. Error : %v", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			endpointsBelongNode[pod.Status.PodIP] = pod.Spec.NodeName
		}
	}
	w.endpoint2Node = endpointsBelongNode
}

func (w *WAOLB) CollectNodeAndPodList() {
	w.GetNodesName()
	w.GetPodsEndpoint()
	klog.Infof("NodesName: %#v", w.nodeNames)
	klog.Infof("Endpoints: %#v", w.endpoint2Node)
}

func (w *WAOLB) CalcNodesScore() {
	piece := len(w.nodeNames)
	workqueue.ParallelizeUntil(context.TODO(), Parallelism, piece, func(piece int) {
		w.nodeScores[w.nodeNames[piece]] = int64(w.Score(w.nodeNames[piece]))
	}, chunkSizeFor(piece))
	klog.Infof("NodesScore: %#v", w.nodeScores)
}

// chunkSizeFor returns a chunk size for the given number of items to use for parallel work.
// See: kubernetes/pkg/scheduler/framework/parallelize
func chunkSizeFor(n int) workqueue.Options {
	s := int(math.Sqrt(float64(n)))
	if r := n/Parallelism + 1; s > r {
		s = r
	} else if s < 1 {
		s = 1
	}
	return workqueue.WithChunkSize(s)
}

// Score calculates node score.
// The returned score is the amount of increase in current power consumption.
func (w *WAOLB) Score(nodeName string) int64 {
	klog.V(5).Infof("%v : Start Score() function", nodeName)

	ctx := context.TODO()

	nodeInfo, err := w.k8sclient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("%v : Cannot get Nodes info. Error : %v", nodeName, err)
		return -1
	}

	// get node metrics
	nodeMetrics, err := w.metricsclient.GetNodeMetrics(ctx, nodeName)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}

	// prepare beforeUsage and afterUsage
	beforeUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()

	//
	nodeResource := nodeInfo.Status.Capacity["cpu"]
	nodeCPUCapacity, _ := strconv.ParseFloat(nodeResource.AsDec().String(), 32)
	cpuCapacity := float64(nodeCPUCapacity)
	klog.InfoS("wao.Score usage", "node", nodeName, "usage_before", beforeUsage)

	beforeUsage = (beforeUsage / cpuCapacity) * 100
	klog.InfoS("wao.Score usage (formatted)", "node", nodeName, "usage_before", beforeUsage, "cpu_capacity", cpuCapacity)

	// get custom metrics
	inletTemp, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	deltaP, err := w.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score metrics", "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := w.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	for _, e := range ncs.Items {
		klog.Infof("e: %v", e)
		// TODO: handle node with multiple NodeConfig
		if e.Spec.NodeName == nodeName {
			nc = e.DeepCopy()
			break
		}
	}
	if nc == nil {
		klog.ErrorS(fmt.Errorf("nodeconfig == nil"), "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
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
			klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
			return -1
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := w.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score prediction", "node", nodeName, "watt_before", beforeWatt)

	return int64(beforeWatt)
}

func (w *WAOLB) CalcModRanges(endpointList []string) (modRanges []int64) {
	if len(endpointList) == 0 {
		return
	}

	minScore := int64(math.MaxInt64)
	for _, ip := range endpointList {
		score, ok := w.nodeScores[w.endpoint2Node[ip]]
		if !ok || score <= 0 {
			continue
		}
		if score < minScore {
			minScore = score
		}
	}
	if minScore == int64(math.MaxInt64) {
		return
	}

	for _, ip := range endpointList {
		score, ok := w.nodeScores[w.endpoint2Node[ip]]
		modRange := int64(0)
		if ok && score > 0 {
			modRange = int64(MaxModRange * minScore / score)
		}
		modRanges = append(modRanges, modRange)
	}
	return
}
