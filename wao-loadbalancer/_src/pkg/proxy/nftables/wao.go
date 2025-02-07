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

	metricsclientv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/custom_metrics"

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
	// parallelism for better CPU utilization,
	// using k8s.io/kubernetes/pkg/scheduler/internal/parallelize as a reference.
	parallelism = 16

	MaxModRange = int64(100)

	DefaultMetricsCacheTTL   = 30 * time.Second
	DefaultPredictorCacheTTL = 30 * time.Minute
)

type Wao struct {
	clientSet           *kubernetes.Clientset
	ctrlclient          client.Client
	nodesName           []string
	endpointsBelongNode map[string]string
	nodesScore          map[string]int64
	metricsclient       *waoclient.CachedMetricsClient
	predictorclient     *waoclient.CachedPredictorClient
}

func NewWao() *Wao {
	var clientSet *kubernetes.Clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Warningf("Cannot get InClusterConfig. Error : %v", err)
	} else {
		clientSet, err = kubernetes.NewForConfig(config)
		if err != nil {
			klog.Warningf("Cannot create new clientSet. Error : %v", err)
		}
	}
	// init metrics client
	mc, err := metricsclientv1beta1.NewForConfig(config)
	if err != nil {
		return nil
	}
	// init custom metrics client
	// https://github.com/kubernetes/kubernetes/blob/7b9d244efd19f0d4cce4f46d1f34a6c7cff97b18/test/e2e/instrumentation/monitoring/custom_metrics_stackdriver.go#L59
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil
	}
	rm := restmapper.NewDeferredDiscoveryRESTMapper(cacheddiscovery.NewMemCacheClient(dc))
	rm.Reset()
	avg := custom_metrics.NewAvailableAPIsGetter(dc)
	cmc := custom_metrics.NewForConfig(config, rm, avg)
	// init controller-runtime client
	scheme := runtime.NewScheme()
	utilruntime.Must(waov1beta1.AddToScheme(scheme))
	ca, err := cache.New(config, cache.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil
	}
	go ca.Start(context.TODO())
	c, err := client.New(config, client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: ca},
	})
	if err != nil {
		return nil
	}
	return &Wao{
		clientSet:           clientSet,
		ctrlclient:          c,
		nodesName:           []string{},
		endpointsBelongNode: make(map[string]string),
		nodesScore:          make(map[string]int64),
		metricsclient:       waoclient.NewCachedMetricsClient(mc, cmc, DefaultMetricsCacheTTL),
		predictorclient:     waoclient.NewCachedPredictorClient(clientSet, DefaultMetricsCacheTTL),
	}
}

// Get list of nodes Name inside Cluster
func (wao *Wao) getNodesName() {
	nodesName := []string{}
	nodes, err := wao.clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
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
	wao.nodesName = nodesName
}

// Get list of pods endpoint inside Cluster
func (wao *Wao) getPodsEndpoint() {
	endpointsBelongNode := make(map[string]string)

	pods, err := wao.clientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Warningf("Cannot get list of pods. Error : %v", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			endpointsBelongNode[pod.Status.PodIP] = pod.Spec.NodeName
		}
	}
	wao.endpointsBelongNode = endpointsBelongNode
}

func (wao *Wao) collectNodeAndPodList() {
	wao.getNodesName()
	wao.getPodsEndpoint()
	klog.Infof("NodesName: %#v", wao.nodesName)
	klog.Infof("Endpoints: %#v", wao.endpointsBelongNode)
}

func (wao *Wao) calcNodesScore() {
	piece := len(wao.nodesName)
	workqueue.ParallelizeUntil(context.TODO(), parallelism, piece, func(piece int) {
		wao.nodesScore[wao.nodesName[piece]] = int64(wao.Score(wao.nodesName[piece]))
	}, chunkSizeFor(piece))
	klog.Infof("NodesScore: %#v", wao.nodesScore)
}

// chunkSizeFor returns a chunk size for the given number of items to use for
// parallel work. The size aims to produce good CPU utilization.
// using k8s.io/kubernetes/pkg/scheduler/internal/parallelize as a reference.
func chunkSizeFor(n int) workqueue.Options {
	s := int(math.Sqrt(float64(n)))
	if r := n/parallelism + 1; s > r {
		s = r
	} else if s < 1 {
		s = 1
	}
	return workqueue.WithChunkSize(s)
}

// Score calculates node score.
// The returned score is the amount of increase in current power consumption.
func (wao *Wao) Score(nodeName string) int64 {
	klog.V(5).Infof("%v : Start Score() function", nodeName)

	ctx := context.TODO()

	nodeInfo, err := wao.clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("%v : Cannot get Nodes info. Error : %v", nodeName, err)
		return -1
	}

	// get node metrics
	nodeMetrics, err := wao.metricsclient.GetNodeMetrics(ctx, nodeName)
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
	inletTemp, err := wao.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	deltaP, err := wao.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score metrics", "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := wao.ctrlclient.List(ctx, &ncs); err != nil {
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
		ep2, err := wao.predictorclient.GetPredictorEndpoint(ctx, nc.Namespace, nc.Spec.Predictor.PowerConsumptionEndpointProvider, predictor.TypePowerConsumption)
		if err != nil {
			klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
			return -1
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := wao.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score prediction", "node", nodeName, "watt_before", beforeWatt)

	return int64(beforeWatt)
}

func (wao *Wao) calcModRanges(endpointList []string) (modRanges []int64) {
	if len(endpointList) == 0 {
		return
	}

	minScore := int64(math.MaxInt64)
	for _, ip := range endpointList {
		score, ok := wao.nodesScore[wao.endpointsBelongNode[ip]]
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
		score, ok := wao.nodesScore[wao.endpointsBelongNode[ip]]
		modRange := int64(0)
		if ok && score > 0 {
			modRange = int64(MaxModRange * minScore / score)
		}
		modRanges = append(modRanges, modRange)
	}
	return
}
