package ipvs

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	waometrics "github.com/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/wao-core/pkg/predictor"
)

const (
	// parallelism for better CPU utilization,
	// using k8s.io/kubernetes/pkg/scheduler/internal/parallelize as a reference.
	parallelism = 16

	// MaxWeight is highest ipvs weight.(1 ~ 65535)
	MaxWeight = 100

	DefaultMetricsCacheTTL   = 30 * time.Second
	DefaultPredictorCacheTTL = 30 * time.Minute
)

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

// CalcWeight calculate endpoints weight
func (proxier *Proxier) CalcWeight(endpointlist []string) map[string]int {
	weight := make(map[string]int)
	klog.V(5).Infof("endpointlist: %v", endpointlist)
	if len(endpointlist) == 0 {
		return weight
	} else if len(endpointlist) == 1 {
		weight[endpointlist[0]] = 1
		return weight
	}

	lowest := int64(math.MaxInt64)
	for _, endpoint := range endpointlist {
		ip, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			klog.Errorf("Failed to parse endpoint: %v, error: %v", endpoint, err)
			continue
		}
		tmpScore, ok := proxier.nodesScore[proxier.endpointsBelongNode[ip]]
		if !ok || tmpScore == -1 {
			continue
		}
		if tmpScore < lowest {
			lowest = tmpScore
		}
	}
	klog.V(5).Infof("Lowest score: %v", lowest)

	// Endpoint weight is larger as nodesScore is smaller.
	for _, endpoint := range endpointlist {
		// Endpoint weight set to 1, if the scores of all nodes could not be calculated.
		if lowest == int64(math.MaxInt64) {
			weight[endpoint] = 1
			continue
		}
		ip, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			klog.Errorf("Failed to parse endpoint: %v, error: %v", endpoint, err)
			continue
		}
		tmpScore, ok := proxier.nodesScore[proxier.endpointsBelongNode[ip]]
		if !ok || tmpScore == -1 {
			weight[endpoint] = 0
			continue
		}
		weight[endpoint] = int(lowest * MaxWeight / tmpScore)
	}
	return weight
}

// Get list of nodes Name inside Cluster
func (proxier *Proxier) getNodesName() {
	nodesName := []string{}
	nodes, err := proxier.clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
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
	proxier.nodesName = nodesName
}

// Get list of pods endpoint inside Cluster
func (proxier *Proxier) getPodsEndpoint() {
	endpointsBelongNode := make(map[string]string)

	pods, err := proxier.clientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Warningf("Cannot get list of pods. Error : %v", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			endpointsBelongNode[pod.Status.PodIP] = pod.Spec.NodeName
		}
	}
	proxier.endpointsBelongNode = endpointsBelongNode
}

// Score calculates node score.
// The returned score is the amount of increase in current power consumption.
func (proxier *Proxier) Score(nodeName string) int64 {
	klog.V(5).Infof("%v : Start Score() function", nodeName)

	ctx := context.TODO()

	nodeInfo, err := proxier.clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("%v : Cannot get Nodes info. Error : %v", nodeName, err)
		return -1
	}

	// get node metrics
	nodeMetrics, err := proxier.metricsclient.GetNodeMetrics(ctx, nodeName)
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
	inletTemp, err := proxier.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueInletTemperature)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	deltaP, err := proxier.metricsclient.GetCustomMetricForNode(ctx, nodeName, waometrics.ValueDeltaPressure)
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score metrics", "node", nodeName, "inlet_temp", inletTemp.Value.AsApproximateFloat64(), "delta_p", deltaP.Value.AsApproximateFloat64())

	// get NodeConfig
	var nc *waov1beta1.NodeConfig
	var ncs waov1beta1.NodeConfigList
	if err := proxier.ctrlclient.List(ctx, &ncs); err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	for _, e := range ncs.Items {
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
		ep2, err := proxier.predictorclient.GetPredictorEndpoint(ctx, nc.Namespace, nc.Spec.Predictor.PowerConsumptionEndpointProvider, predictor.TypePowerConsumption)
		if err != nil {
			klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
			return -1
		}
		ep.Type = ep2.Type
		ep.Endpoint = ep2.Endpoint
	}

	// do predict
	beforeWatt, err := proxier.predictorclient.PredictPowerConsumption(ctx, nc.Namespace, ep, beforeUsage, inletTemp.Value.AsApproximateFloat64(), deltaP.Value.AsApproximateFloat64())
	if err != nil {
		klog.ErrorS(err, "wao.Score score=ScoreError as error occurred", "node", nodeName)
		return -1
	}
	klog.InfoS("wao.Score prediction", "node", nodeName, "watt_before", beforeWatt)

	return int64(beforeWatt)
}
