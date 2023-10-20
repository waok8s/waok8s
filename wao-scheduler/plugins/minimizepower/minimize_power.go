package minimizepower

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	tfcoreframework "tensorflow/core/framework"
	pb "tensorflow_serving/apis"
	"time"

	google_protobuf "github.com/golang/protobuf/ptypes/wrappers"
	dto "github.com/prometheus/client_model/go"
	p2j "github.com/prometheus/prom2json"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	// CPU usage threshold of a pod that is against the CPU limits.
	workThreshold = 0.50
	// Rate of the CPU limits used to predict CPU usage.
	resourceDecay = 0.85

	// Log level
	debug = 4
	trace = 5

	// Interval to get the temperature. (minutes)
	getTemperatureInterval = 15

	// The score to use if all nodes have the same score.
	tempScore = 10

	// Node label that TensorFlow Serving host.
	labelTensorflowHost = "tensorflow/host"
	// Node label that TensorFlow Serving port.
	labelTensorflowPort = "tensorflow/port"
	// Node label that TensorFlow Serving model name.
	labelTensorflowName = "tensorflow/name"
	// Node label that TensorFlow Serving model version. (optional label)
	labelTensorflowVersion = "tensorflow/version"
	// Node label that TensorFlow Serving model signature. (optional label)
	labelTensorflowSignature = "tensorflow/signature"

	// Node label that max ambient temperature.
	labelAmbientMax = "ambient/max"
	// Node label that min ambient temperature.
	labelAmbientMin = "ambient/min"
	// Node label that cpu1 max information.
	labelCPU1Max = "cpu1/max"
	// Node label that cpu1 min information.
	labelCPU1Min = "cpu1/min"
	// Node label that cpu2 max information.
	labelCPU2Max = "cpu2/max"
	// Node label that cpu2 min information.
	labelCPU2Min = "cpu2/min"

	// IPMI exporter port
	ipmiPort = "9290"
	// IPMI exporter protocol
	ipmiProtocol = "http://"
	// IPMI exporter metrics endpoint
	endpoint = "/metrics"
	// IPMI exporter temperature metrics key
	ipmiTemperatureKey = "ipmi_temperature_celsius"
	// IPMI exporter ambient temperature key
	ambientKey = "Ambient"
	// IPMI exporter CPU1 temperature key
	cpu1Key = "CPU1"
	// IPMI exporter CPU2 temperature key
	cpu2Key = "CPU2"
)

// OsmoticComputingOptimizer is a plugin that prioritizes the nodes with the lowest increase in power consumption.
type OsmoticComputingOptimizer struct {
	handle                framework.Handle
	ambient               map[string]float32
	cpu1                  map[string]float32
	cpu2                  map[string]float32
	ambientTimestamp      map[string]time.Time
	powerConsumptionCache map[cacheKey]float32
	startingPodCPU        map[string]float64
	getInfo               getInfomations
	sync.Mutex
}

// familyInfo defines IPMI exporter metrics objects.
type familyInfo struct {
	Name    string `json:"name"`
	Metrics []struct {
		Labels struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		Value string `json:"value"`
	} `json:"metrics"`
}

// cacheKey defines the key to use when caching power consumption predictions.
type cacheKey struct {
	server       string
	predictInput predictInput
}

// predictInput defines the input for power consumption predictions.
type predictInput struct {
	cpuUsage    float32
	ambientTemp float32
	cpu1Temp    float32
	cpu2Temp    float32
}

// getInfomationsImpl implements the getInfomations interface.
type getInfomationsImpl struct{}

// getInfomations is the interface to get information.
type getInfomations interface {
	// Get node metrics from metrics server.
	getNodeMetrics(ctx context.Context, node string) (*v1beta1.NodeMetrics, error)
	// Get pod metrics from metrics server.
	getPodMetrics(ctx context.Context, pod string, namespace string) (*v1beta1.PodMetrics, error)
	// Get node sensor data from IPMI exporter.
	getFamilyInfo(url string) []*p2j.Family
	// Get prediction power consumption from TensorFlow Serving.
	predictPC(values predictInput, nodeInfo *framework.NodeInfo) (float32, error)
}

var _ = framework.ScorePlugin(&OsmoticComputingOptimizer{})

// MinimizePowerName is the name of the plugin used in the plugin registry and configurations.
const MinimizePowerName = "oc-score-plugin"

// Name returns name of the plugin. It is used in logs, etc.
func (oco *OsmoticComputingOptimizer) Name() string {
	return MinimizePowerName
}

// Get node metrics from metrics server.
func (i getInfomationsImpl) getNodeMetrics(ctx context.Context, node string) (*v1beta1.NodeMetrics, error) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.V(trace).Infof("%v : Cannot get metrics infomations because %v", node, err)
		return nil, err
	}
	mc, err := metrics.NewForConfig(config)
	if err != nil {
		klog.V(trace).Infof("%v : Cannot get metrics infomations because %v", node, err)
		return nil, err
	}
	nm := mc.MetricsV1beta1().NodeMetricses()
	nodemetrics, err := nm.Get(ctx, node, metav1.GetOptions{})
	klog.V(trace).Infof("%v : Nodemetrics is %+v", node, nodemetrics)
	return nodemetrics, err
}

// Get pod metrics from metrics server.
func (i getInfomationsImpl) getPodMetrics(ctx context.Context, pod string, namespace string) (*v1beta1.PodMetrics, error) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.V(trace).Infof("%v/%v : Cannot get metrics infomations because %v", namespace, pod, err)
		return nil, err
	}
	mc, err := metrics.NewForConfig(config)
	if err != nil {
		klog.V(trace).Infof("%v/%v : Cannot get metrics infomations because %v", namespace, pod, err)
		return nil, err
	}
	pm := mc.MetricsV1beta1().PodMetricses(namespace)
	podmetrics, err := pm.Get(ctx, pod, metav1.GetOptions{})
	klog.V(trace).Infof("%v/%v : Podmetrics is %+v", namespace, pod, podmetrics)
	return podmetrics, err
}

// Get node sensor data from IPMI exporter.
func (i getInfomationsImpl) getFamilyInfo(url string) []*p2j.Family {
	mfChan := make(chan *dto.MetricFamily, 1024)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	go func() {
		err := p2j.FetchMetricFamilies(url, mfChan, transport)
		if err != nil {
			klog.V(trace).Infof("Cannot get metric from %v because %v", url, err)
		}
	}()

	result := []*p2j.Family{}
	for mf := range mfChan {
		result = append(result, p2j.NewFamily(mf))
	}
	return result
}

// Get prediction power consumption from TensorFlow Serving.
func (i getInfomationsImpl) predictPC(pi predictInput, nodeInfo *framework.NodeInfo) (float32, error) {
	values := []float32{pi.cpuUsage, pi.ambientTemp, pi.cpu1Temp, pi.cpu2Temp}

	servingAddress, hostOK := nodeInfo.Node().Labels[labelTensorflowHost]
	servingPort, portOK := nodeInfo.Node().Labels[labelTensorflowPort]
	name, nameOK := nodeInfo.Node().Labels[labelTensorflowName]
	if !hostOK || !portOK || !nameOK {
		return -1, fmt.Errorf("Label is not defined. [%v: %v, %v: %v, %v: %v]",
			labelTensorflowHost, hostOK, labelTensorflowPort, portOK, labelTensorflowName, nameOK)
	}

	request := &pb.PredictRequest{
		ModelSpec: &pb.ModelSpec{
			Name: name,
		},
		Inputs: map[string]*tfcoreframework.TensorProto{
			"inputs": {
				Dtype: tfcoreframework.DataType_DT_FLOAT,
				TensorShape: &tfcoreframework.TensorShapeProto{
					Dim: []*tfcoreframework.TensorShapeProto_Dim{
						{
							Size: int64(1),
						},
						{
							Size: int64(4),
						},
					},
				},
				FloatVal: values,
			},
		},
	}
	if strVersion, ok := nodeInfo.Node().Labels[labelTensorflowVersion]; ok {
		if intVersion, err := strconv.ParseInt(strVersion, 10, 64); err == nil {
			request.ModelSpec.Version = &google_protobuf.Int64Value{Value: intVersion}
		} else {
			klog.Warningf("Convert to int64 failed [%v : %v]", labelTensorflowVersion, strVersion)
		}
	}
	if signature, ok := nodeInfo.Node().Labels[labelTensorflowSignature]; ok {
		request.ModelSpec.SignatureName = signature
	}

	conn, err := grpc.Dial(servingAddress+":"+servingPort, grpc.WithInsecure())
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	client := pb.NewPredictionServiceClient(conn)

	resp, err := client.Predict(context.Background(), request)
	if err != nil {
		return -1, err
	}
	return resp.Outputs["outputs"].FloatVal[0], nil
}

// Get node Ambient and CPU1 and CPU2 temperatures used to predict power consumption.
func (oco *OsmoticComputingOptimizer) getNodeTemperature(nodeAddress string) ([]float32, error) {
	var url string = strings.Join([]string{ipmiProtocol, nodeAddress, ":", ipmiPort, endpoint}, "")
	result := oco.getInfo.getFamilyInfo(url)
	if len(result) == 0 {
		return nil, fmt.Errorf("Cannot get FamilyInfo")
	}

	var families []familyInfo
	jsonText, err := json.Marshal(result)
	if err != nil {
		klog.V(trace).Infof("Marshal cannot encoding from %v because %v", nodeAddress, err)
		return nil, err
	}
	if err := json.Unmarshal(jsonText, &families); err != nil {
		klog.V(trace).Infof("JSON-encoded data cannot be parsed because %v", err)
		return nil, err
	}

	for _, f := range families {
		if f.Name == ipmiTemperatureKey {
			ambient, CPU1, CPU2 := float64(-1), float64(-1), float64(-1)
			var ambientErr, CPU1Err, CPU2Err error
			for _, m := range f.Metrics {
				if m.Labels.Name == ambientKey {
					ambient, ambientErr = strconv.ParseFloat(m.Value, 32)
				}
				if m.Labels.Name == cpu1Key {
					CPU1, CPU1Err = strconv.ParseFloat(m.Value, 32)
				}
				if m.Labels.Name == cpu2Key {
					CPU2, CPU2Err = strconv.ParseFloat(m.Value, 32)
				}

				if ambient != -1 && CPU1 != -1 && CPU2 != -1 {
					break
				}
			}

			if ambientErr != nil || CPU1Err != nil || CPU2Err != nil {
				klog.V(trace).Infof("Convert to float64 failed from %v [Ambient : %v, CPU1 : %v, CPU2 : %v]", nodeAddress, ambientErr, CPU1Err, CPU2Err)
				return nil, fmt.Errorf("Convert to float64 failed")
			}
			if ambient == -1 || CPU1 == -1 || CPU2 == -1 {
				klog.V(trace).Infof("Cannot get node temperature from %v [Ambient : %v, CPU1 : %v, CPU2 : %v]", nodeAddress, ambient, CPU1, CPU2)
				return nil, fmt.Errorf("Cannot get node temperature")
			}
			return []float32{float32(ambient), float32(CPU1), float32(CPU2)}, nil
		}
	}

	klog.V(trace).Infof("%v is not exits from %v", ipmiTemperatureKey, nodeAddress)
	return nil, fmt.Errorf("%v is not exits from %v", ipmiTemperatureKey, nodeAddress)
}

// Set predicted power consumption to the cache.
func (oco *OsmoticComputingOptimizer) setPCCache(k cacheKey, value float32) {
	oco.Lock()
	defer oco.Unlock()
	oco.powerConsumptionCache[k] = value
}

// Get predicted power consumption from the cache.
func (oco *OsmoticComputingOptimizer) getPCCache(k cacheKey) (float32, bool) {
	oco.Lock()
	defer oco.Unlock()
	value, ok := oco.powerConsumptionCache[k]
	return value, ok
}

// Delete from the list of the amount of CPU used by the pod.
// Adding to the list from NormalizeScore function.
func (oco *OsmoticComputingOptimizer) deletePodCPU(k string) {
	oco.Lock()
	defer oco.Unlock()
	delete(oco.startingPodCPU, k)
}

// Get the amount of CPU used by the pod.
func (oco *OsmoticComputingOptimizer) getPodCPU(k string) (float64, bool) {
	oco.Lock()
	defer oco.Unlock()
	v, ok := oco.startingPodCPU[k]
	return v, ok
}

// Get the node's internal IP for the IPMI exporter's address.
func getNodeInternalIP(node *v1.Node) (string, error) {
	for _, addres := range node.Status.Addresses {
		if addres.Type == v1.NodeInternalIP {
			return addres.Address, nil
		}
	}
	return "", errors.New("Cannot get node internalIP")
}

// Get standard temperature informations from the node's label.
func getStandardTemperature(nodeInfo *framework.NodeInfo, labelKeyMax string, labelKeyMin string) (float32, float32, error) {
	tempMax, maxOK := nodeInfo.Node().Labels[labelKeyMax]
	tempMin, minOK := nodeInfo.Node().Labels[labelKeyMin]
	if !maxOK || !minOK {
		klog.V(trace).Infof("%v : Standard temperature information label is not defined. [%v : %v, %v : %v] ",
			nodeInfo.Node().Name, labelKeyMax, maxOK, labelKeyMin, minOK)
		return -1, -1, errors.New("Standard temperature informations label is not defined")
	}

	maxValue, err := strconv.ParseFloat(tempMax, 32)
	if err != nil {
		klog.V(trace).Infof("Convert to float64 failed. [%v : %v]", labelKeyMax, tempMax)
		return -1, -1, errors.New("Convert to float64 failed")
	}

	minValue, err := strconv.ParseFloat(tempMin, 32)
	if err != nil {
		klog.V(trace).Infof("Convert to float64 failed. [%v : %v]", labelKeyMin, tempMin)
		return -1, -1, errors.New("Convert to float64 failed")
	}

	// The standard temperature informations max and min must not be the same because division by zero occurs in calcNormalizeTemperature.
	if (maxValue - minValue) == 0 {
		klog.V(trace).Infof("%v : Do not set values of %v and %v the same", nodeInfo.Node().Name, labelKeyMax, labelKeyMin)
		return -1, -1, errors.New("division by zero")
	}
	return float32(maxValue), float32(minValue), nil
}

// Calclate node CPU usage
func (oco *OsmoticComputingOptimizer) calcCPUUsage(ctx context.Context, nodeName string, nodeInfo *framework.NodeInfo, pod *v1.Pod) (float32, float32, *framework.Status) {
	klog.V(trace).Infof("%v : Start calcCPUUsage() function", nodeName)

	nodeMetrics, err := oco.getInfo.getNodeMetrics(ctx, nodeName)
	if err != nil {
		klog.Errorf("%v : Cannot get metrics infomations becouse %v", nodeName, err)
		return -1, -1, framework.NewStatus(framework.Error, fmt.Sprintf("Metrics of node %v cannot be got", nodeName))
	}
	nodeMetricsCPU := nodeMetrics.Usage["cpu"]
	nodeCPUUsage, _ := strconv.ParseFloat(nodeMetricsCPU.AsDec().String(), 32)
	nodeResource := nodeInfo.Node().Status.Capacity["cpu"]
	nodeCPUCapacity, _ := strconv.ParseFloat(nodeResource.AsDec().String(), 32)

	for _, podInfo := range nodeInfo.Pods {
		if v, ok := oco.getPodCPU(podInfo.Pod.ObjectMeta.Name); ok {
			var podUsage float64
			podMetrics, err := oco.getInfo.getPodMetrics(ctx, podInfo.Pod.ObjectMeta.Name, podInfo.Pod.ObjectMeta.Namespace)
			if err == nil {
				for _, container := range podMetrics.Containers {
					containerCPU := container.Usage["cpu"]
					containerUsage, _ := strconv.ParseFloat(containerCPU.AsDec().String(), 32)
					podUsage += containerUsage
				}
			}
			if podUsage < v*workThreshold {
				nodeCPUUsage += v - podUsage
			} else {
				oco.deletePodCPU(podInfo.Pod.ObjectMeta.Name)
			}
		}
	}

	podResourceLimits := pod.Spec.Containers[0].Resources.Limits["cpu"]
	limitsCore, errLimit := strconv.ParseFloat(podResourceLimits.AsDec().String(), 32)
	podResourceRequests := pod.Spec.Containers[0].Resources.Requests["cpu"]
	requestsCore, errRequest := strconv.ParseFloat(podResourceRequests.AsDec().String(), 32)

	if (limitsCore == 0 || errLimit != nil) && (requestsCore == 0 || errRequest != nil) {
		klog.Warningf("Pod %v does not define requested and limited resources", pod.Name)
		return -1, -1, nil
	}

	if (nodeCPUUsage + limitsCore) > nodeCPUCapacity {
		klog.V(trace).Infof("%v : Score is -1 because overload", nodeName)
		return -1, -1, nil
	}

	var afterUsage float32
	beforeUsage := float32(nodeCPUUsage / nodeCPUCapacity)
	klog.V(trace).Infof("%v : CPUusage %v", nodeName, beforeUsage)

	if requestsCore != 0 {
		afterUsage = float32((nodeCPUUsage + requestsCore) / nodeCPUCapacity)
	} else {
		afterUsage = float32((nodeCPUUsage + limitsCore*resourceDecay) / nodeCPUCapacity)
	}

	return beforeUsage, afterUsage, nil
}

// Calculate normalized node temperature.
func (oco *OsmoticComputingOptimizer) calcNormalizeTemperature(nodeName string, nodeInfo *framework.NodeInfo) (float32, float32, float32, error) {
	klog.V(trace).Infof("%v : Start calcNormalizeTemperature() function", nodeName)
	ambientMax, ambientMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)
	if err != nil {
		return -1, -1, -1, err
	}

	CPU1Max, CPU1Min, err := getStandardTemperature(nodeInfo, labelCPU1Max, labelCPU1Min)
	if err != nil {
		return -1, -1, -1, err
	}

	CPU2Max, CPU2Min, err := getStandardTemperature(nodeInfo, labelCPU2Max, labelCPU2Min)
	if err != nil {
		return -1, -1, -1, err
	}

	if oco.ambientTimestamp[nodeName].IsZero() || int(time.Since(oco.ambientTimestamp[nodeName]).Minutes()) >= getTemperatureInterval {
		nodeIPAddress, err := getNodeInternalIP(nodeInfo.Node())
		if err == nil {
			temp, err := oco.getNodeTemperature(nodeIPAddress)
			if err == nil {
				oco.ambient[nodeName] = temp[0]
				oco.cpu1[nodeName] = temp[1]
				oco.cpu2[nodeName] = temp[2]
				oco.ambientTimestamp[nodeName] = time.Now()
			}
		} else {
			klog.Warningf("%v : Cannot get InternalIP.", nodeName)
		}

	}
	_, okAmb := oco.ambient[nodeName]
	_, okCPU1 := oco.cpu1[nodeName]
	_, okCPU2 := oco.cpu2[nodeName]
	if !okAmb || !okCPU1 || !okCPU2 {
		klog.Warningf("%v : not exist ipmi data", nodeName)
		return -1, -1, -1, errors.New("not exist ipmi data")
	}
	klog.V(trace).Infof("%v : temperature [ambient: %v ℃, CPU1: %v ℃, CPU2: %v ℃]", nodeName, oco.ambient[nodeName], oco.cpu1[nodeName], oco.cpu2[nodeName])

	normalizedAmbient := (oco.ambient[nodeName] - ambientMin) / (ambientMax - ambientMin)
	normalizedCPU1 := (oco.cpu1[nodeName] - CPU1Min) / (CPU1Max - CPU1Min)
	normalizedCPU2 := (oco.cpu2[nodeName] - CPU2Min) / (CPU2Max - CPU2Min)

	return normalizedAmbient, normalizedCPU1, normalizedCPU2, nil
}

// Calculate the score from CPU usage and temperature rise rate.
func (oco *OsmoticComputingOptimizer) calcScore(nodeName string, infoBefore predictInput, infoAfter predictInput, nodeInfo *framework.NodeInfo) int64 {
	klog.V(trace).Infof("%v : Start calcScore() function", nodeName)
	var pcBefore, pcAfter float32
	var err error
	keyBefore := cacheKey{nodeName, infoBefore}
	if v, ok := oco.getPCCache(keyBefore); ok && v != -1 {
		klog.V(trace).Infof("%v : Use cache %+v=%v", nodeName, keyBefore, v)
		pcBefore = v
	} else {
		pcBefore, err = oco.getInfo.predictPC(infoBefore, nodeInfo)
		if err != nil {
			klog.Warningf("%v : Cannnot predict before power consumption because %v", nodeName, err)
			return -1
		}
		klog.V(trace).Infof("%v : predictPC result %+v=%v", nodeName, keyBefore, pcBefore)
		oco.setPCCache(keyBefore, pcBefore)
	}
	keyAfter := cacheKey{nodeName, infoAfter}
	if v, ok := oco.getPCCache(keyAfter); ok && v != -1 {
		klog.V(trace).Infof("%v : Use cache %+v=%v", nodeName, keyAfter, v)
		pcAfter = v
	} else {
		pcAfter, err = oco.getInfo.predictPC(infoAfter, nodeInfo)
		if err != nil {
			klog.Warningf("%v : Cannnot predict after power consumption because %v", nodeName, err)
			return -1
		}
		klog.V(trace).Infof("%v : predictPC result %+v=%v", nodeName, keyAfter, pcAfter)
		oco.setPCCache(keyAfter, pcAfter)
	}

	return int64(pcAfter - pcBefore)
}

// Score invoked at the score extension point.
// The returned score is the amount of increase in power consumption.
func (oco *OsmoticComputingOptimizer) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	klog.V(trace).Infof("%v : Start Score() function", nodeName)
	nodeInfo, err := oco.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		klog.Errorf("%v : Cannnot get nodeInfo because %v", nodeName, err)
		return -1, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %v from Snapshot: %v", nodeName, err))
	}
	if nodeInfo.Node() == nil {
		klog.Errorf("%v : nodeInfo is nil", nodeName)
		return -1, framework.NewStatus(framework.Error, fmt.Sprintf("Node %v infomations is nil", nodeName))
	}

	beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(ctx, nodeName, nodeInfo, pod)
	if calcCPUUsageErr != nil {
		return -1, calcCPUUsageErr
	} else if beforeUsage == -1 && afterUsage == -1 {
		return -1, nil
	}

	var beforeInput, afterInput predictInput
	ambientTemp, cpu1Temp, cpu2Temp, err := oco.calcNormalizeTemperature(nodeName, nodeInfo)
	if err != nil {
		return -1, nil
	}
	beforeInput.ambientTemp = float32(math.Round(float64(ambientTemp)*10) / 10)
	beforeInput.cpu1Temp = float32(math.Round(float64(cpu1Temp)*10) / 10)
	beforeInput.cpu2Temp = float32(math.Round(float64(cpu2Temp)*10) / 10)
	afterInput = beforeInput
	beforeInput.cpuUsage = float32(math.Round(float64(beforeUsage)*10) / 10)
	afterInput.cpuUsage = float32(math.Round(float64(afterUsage)*10) / 10)

	klog.V(trace).Infof("%v : before predict params %+v", nodeName, beforeInput)
	klog.V(trace).Infof("%v : after predict params %+v", nodeName, afterInput)

	return oco.calcScore(nodeName, beforeInput, afterInput, nodeInfo), nil
}

// ScoreExtensions of the Score plugin.
func (oco *OsmoticComputingOptimizer) ScoreExtensions() framework.ScoreExtensions {
	return oco
}

// NormalizeScore invoked after scoring all nodes.
func (oco *OsmoticComputingOptimizer) NormalizeScore(ctx context.Context, state *framework.CycleState, p *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	klog.V(debug).Infof("%v : ScoreList %v", p.Name, scores)
	highest := int64(0)
	lowest := int64(math.MaxInt64)
	for _, score := range scores {
		if score.Score > highest {
			highest = score.Score
		}
		if score.Score < lowest && score.Score >= 0 {
			lowest = score.Score
		}
	}
	nodeScoreMax := framework.MaxNodeScore
	for node, score := range scores {
		if score.Score < 0 {
			scores[node].Score = 0
		} else if highest != lowest {
			scores[node].Score = int64(nodeScoreMax - (nodeScoreMax * (score.Score - lowest) / (highest - lowest)))
		} else {
			scores[node].Score = tempScore
		}
	}
	klog.V(debug).Infof("%v : NormalizedScoreList %v", p.Name, scores)

	// Set the list of prediction amount of CPU used by the pod.
	podResourceLimits := p.Spec.Containers[0].Resources.Limits["cpu"]
	limitsCore, err := strconv.ParseFloat(podResourceLimits.AsDec().String(), 32)
	if limitsCore != 0 && err == nil {
		oco.startingPodCPU[p.Name] = limitsCore * resourceDecay
		return nil
	}
	podResourceRequests := p.Spec.Containers[0].Resources.Requests["cpu"]
	requestsCore, err := strconv.ParseFloat(podResourceRequests.AsDec().String(), 32)
	if requestsCore != 0 && err == nil {
		oco.startingPodCPU[p.Name] = requestsCore
	}
	return nil
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	var oco OsmoticComputingOptimizer
	oco.handle = h
	oco.powerConsumptionCache = make(map[cacheKey]float32)
	oco.ambient = make(map[string]float32)
	oco.cpu1 = make(map[string]float32)
	oco.cpu2 = make(map[string]float32)
	oco.ambientTimestamp = make(map[string]time.Time)
	oco.startingPodCPU = make(map[string]float64)
	oco.getInfo = getInfomationsImpl{}
	klog.V(trace).Infof("New OsmoticComputingOptimizer %+v", &oco)
	return &oco, nil
}
