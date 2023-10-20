package minimizepower

import (
	"context"
	"errors"
	// "fmt"
	"testing"
	"time"

	p2j "github.com/prometheus/prom2json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgruntime "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	// "k8s.io/kubernetes/pkg/scheduler/internal/cache"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// mockGetInformations implements the getInfomations interface for mocking.
type mockGetInformations struct {
	mock.Mock
}

// Mock getNodeMetrics function.
func (mock *mockGetInformations) getNodeMetrics(ctx context.Context, node string) (*v1beta1.NodeMetrics, error) {
	results := mock.Called()
	return results.Get(0).(*v1beta1.NodeMetrics), results.Error(1)
}

// Mock getPodMetrics function.
func (mock *mockGetInformations) getPodMetrics(ctx context.Context, pod string, namespace string) (*v1beta1.PodMetrics, error) {
	results := mock.Called()
	return results.Get(0).(*v1beta1.PodMetrics), results.Error(1)
}

// Mock getFamilyInfo function.
func (mock *mockGetInformations) getFamilyInfo(url string) []*p2j.Family {
	results := mock.Called()
	return results.Get(0).([]*p2j.Family)
}

// Mock predictPC function.
func (mock *mockGetInformations) predictPC(values predictInput, nodeInfo *framework.NodeInfo) (float32, error) {
	results := mock.Called()
	return results.Get(0).(float32), results.Error(1)
}

// Test for getFamilyInfo
// Do not normal case tests because it needs a connecting server.
func TestGetFamilyInfo(t *testing.T) {
	assert := assert.New(t)
	t.Run("Abnormal case [GET request executing failed]", func(t *testing.T) {
		var getInfo getInfomationsImpl
		result := getInfo.getFamilyInfo("http://255.255.255.255:9290/metrics")
		assert.Equal(result, []*p2j.Family{})
	})
}

// Test for predictPC
// Do not normal case tests because it needs a connecting server.
func TestPredictPC(t *testing.T) {
	assert := assert.New(t)
	var oco OsmoticComputingOptimizer
	var h framework.Handle
	oco.handle = h
	oco.powerConsumptionCache = make(map[cacheKey]float32)
	oco.ambient = make(map[string]float32)
	oco.cpu1 = make(map[string]float32)
	oco.cpu2 = make(map[string]float32)
	oco.ambientTimestamp = make(map[string]time.Time)
	oco.startingPodCPU = make(map[string]float64)

	testNodes := struct {
		node []*v1.Node
	}{
		node: []*v1.Node{
			makeNode("node-1", 30000, 20000, map[string]string{}),
			makeNode("node-2", 30000, 20000, map[string]string{
				"tensorflow/host": "255.255.255.255", "tensorflow/name": "test"}),
			makeNode("node-3", 30000, 20000, map[string]string{
				"tensorflow/host": "255.255.255.255", "tensorflow/port": "8500", "tensorflow/name": "test", "tensorflow/version": "1", "tensorflow/signature": "test"}),
		},
	}

	t.Run("Abnormal case [all label is not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])

		oco.getInfo = getInfomationsImpl{}
		powerConsumpsion, err := oco.getInfo.predictPC(predictInput{}, nodeInfo)

		assert.Equal(powerConsumpsion, float32(-1))
		assert.EqualError(err, "Label is not defined. [tensorflow/host: false, tensorflow/port: false, tensorflow/name: false]")
	})

	t.Run("Abnormal case [tensorflow/port label is not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[1])

		oco.getInfo = getInfomationsImpl{}
		powerConsumpsion, err := oco.getInfo.predictPC(predictInput{}, nodeInfo)

		assert.Equal(powerConsumpsion, float32(-1))
		assert.EqualError(err, "Label is not defined. [tensorflow/host: true, tensorflow/port: false, tensorflow/name: true]")
	})

	t.Run("Abnormal case [Cannot connect server]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[2])

		oco.getInfo = getInfomationsImpl{}
		powerConsumpsion, err := oco.getInfo.predictPC(predictInput{}, nodeInfo)

		assert.Equal(powerConsumpsion, float32(-1))
		assert.Error(err)
	})
}

// Test for getNodeTemperature
func TestGetNodeTemperature(t *testing.T) {
	var oco OsmoticComputingOptimizer
	assert := assert.New(t)

	t.Run("Normal case", func(t *testing.T) {
		mockObj := new(mockGetInformations)
		testData := []*p2j.Family{
			{
				Name: "ipmi_temperature_celsius",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{"id": "1", "name": "Ambient"},
						TimestampMs: "",
						Value:       "20",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "10", "name": "CPU2"},
						TimestampMs: "",
						Value:       "56",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "9", "name": "CPU1"},
						TimestampMs: "",
						Value:       "42",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "99", "name": "testkey"},
						TimestampMs: "",
						Value:       "99",
					},
				},
			},
			{
				Name: "test_data",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{},
						TimestampMs: "",
						Value:       "13888",
					},
				},
			},
		}
		mockObj.On("getFamilyInfo", mock.Anything).Return(testData)
		oco.getInfo = mockObj
		result, err := oco.getNodeTemperature("0.0.0.0")
		assert.Nil(err)
		assert.Equal(result, []float32{20, 42, 56})
	})

	t.Run("Abnormal case [Can't convert to float64]", func(t *testing.T) {
		mockObj := new(mockGetInformations)
		testData := []*p2j.Family{
			{
				Name: "ipmi_temperature_celsius",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{"id": "1", "name": "Ambient"},
						TimestampMs: "",
						Value:       "20",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "10", "name": "CPU2"},
						TimestampMs: "",
						Value:       "56",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "9", "name": "CPU1"},
						TimestampMs: "",
						Value:       "42°C",
					},
				},
			},
		}
		mockObj.On("getFamilyInfo", mock.Anything).Return(testData)
		oco.getInfo = mockObj
		result, err := oco.getNodeTemperature("0.0.0.0")
		assert.EqualError(err, "Convert to float64 failed")
		assert.Nil(result)
	})

	t.Run("Abnormal case [CPU1 information does not exist]", func(t *testing.T) {
		mockObj := new(mockGetInformations)
		testData := []*p2j.Family{
			{
				Name: "ipmi_temperature_celsius",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{"id": "1", "name": "Ambient"},
						TimestampMs: "",
						Value:       "20",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "10", "name": "CPU2"},
						TimestampMs: "",
						Value:       "56",
					},
				},
			},
		}
		mockObj.On("getFamilyInfo", mock.Anything).Return(testData)
		oco.getInfo = mockObj
		result, err := oco.getNodeTemperature("0.0.0.0")
		assert.EqualError(err, "Cannot get node temperature")
		assert.Nil(result)
	})

	t.Run("Abnormal case [ipmi_temperature_celsius key does not exist]", func(t *testing.T) {
		mockObj := new(mockGetInformations)
		testData := []*p2j.Family{
			{
				Name: "ipmi_temperature_test",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{"id": "1", "name": "Ambient"},
						TimestampMs: "",
						Value:       "20",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "10", "name": "CPU2"},
						TimestampMs: "",
						Value:       "56",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "9", "name": "CPU1"},
						TimestampMs: "",
						Value:       "42",
					},
				},
			},
		}
		mockObj.On("getFamilyInfo", mock.Anything).Return(testData)
		oco.getInfo = mockObj
		result, err := oco.getNodeTemperature("0.0.0.0")
		assert.EqualError(err, "ipmi_temperature_celsius is not exits from 0.0.0.0")
		assert.Nil(result)
	})

	t.Run("Abnormal case [GetFamilyInfo method return error]", func(t *testing.T) {
		mockObj := new(mockGetInformations)
		mockObj.On("getFamilyInfo", mock.Anything).Return([]*p2j.Family{})
		oco.getInfo = mockObj
		result, err := oco.getNodeTemperature("0.0.0.0")
		assert.EqualError(err, "Cannot get FamilyInfo")
		assert.Nil(result)
	})
}

// Test for getNodeInternalIP
func TestGetNodeInternalIP(t *testing.T) {
	assert := assert.New(t)
	testNodes := []struct {
		pod   []*v1.Pod
		nodes []*v1.Node
	}{
		{
			pod:   []*v1.Pod{makePod("node-test1", "pod-test1", "1000m", "1000m")},
			nodes: []*v1.Node{makeNode("node-test1", 30000, 20000, map[string]string{})},
		},
		{
			pod:   []*v1.Pod{makePod("node-test2", "pod-test2", "1000m", "1000m")},
			nodes: []*v1.Node{makeNode("node-test2", 30000, 20000, map[string]string{})},
		},
	}

	t.Run("Normal case", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[0].nodes[0])
		nodeInfo.Node().Status.Addresses = append(nodeInfo.Node().Status.Addresses, v1.NodeAddress{
			Type:    v1.NodeInternalIP,
			Address: "11.11.11.11",
		})
		adress, err := getNodeInternalIP(nodeInfo.Node())
		assert.Nil(err)
		assert.Equal(adress, "11.11.11.11")
	})

	t.Run("Abnormal case [Can't get the internal IP of a node.]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[1].nodes[0])
		nodeInfo.Node().Status.Addresses = append(nodeInfo.Node().Status.Addresses, v1.NodeAddress{
			Type:    v1.NodeExternalIP,
			Address: "22.22.22.22",
		})
		adress, err := getNodeInternalIP(nodeInfo.Node())
		assert.EqualError(err, "Cannot get node internalIP")
		assert.Empty(adress)
	})
}

func TestGetStandardTemperature(t *testing.T) {
	assert := assert.New(t)
	testNodes := struct {
		node []*v1.Node
	}{
		node: []*v1.Node{
			makeNode("node-test1", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5"}),
			makeNode("node-test2", 30000, 20000, map[string]string{}),
			makeNode("node-test3", 30000, 20000, map[string]string{"ambient/max": "36.5"}),
			makeNode("node-test4", 30000, 20000, map[string]string{"ambient/max": "36.5°C", "ambient/min": "11.5"}),
			makeNode("node-test5", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5°C"}),
			makeNode("node-test6", 30000, 20000, map[string]string{"ambient/max": "25.5", "ambient/min": "25.5"}),
		},
	}

	t.Run("Normal case", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.Nil(err)
		assert.Equal(ambMax, float32(36.5))
		assert.Equal(ambMin, float32(11.5))
	})

	t.Run("Abnormal case [temperature max and temperature min labels are not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[1])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.EqualError(err, "Standard temperature informations label is not defined")
		assert.Equal(ambMax, float32(-1))
		assert.Equal(ambMin, float32(-1))
	})

	t.Run("Abnormal case [temperature min label is not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[2])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.EqualError(err, "Standard temperature informations label is not defined")
		assert.Equal(ambMax, float32(-1))
		assert.Equal(ambMin, float32(-1))
	})

	t.Run("Abnormal case [temperature max can't convert to float64.]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[3])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.EqualError(err, "Convert to float64 failed")
		assert.Equal(ambMax, float32(-1))
		assert.Equal(ambMin, float32(-1))
	})

	t.Run("Abnormal case [temperature min can't convert to float64.]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[4])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.EqualError(err, "Convert to float64 failed")
		assert.Equal(ambMax, float32(-1))
		assert.Equal(ambMin, float32(-1))
	})

	t.Run("Abnormal case [Occur division by zero.]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[5])
		ambMax, ambMin, err := getStandardTemperature(nodeInfo, labelAmbientMax, labelAmbientMin)

		assert.EqualError(err, "division by zero")
		assert.Equal(ambMax, float32(-1))
		assert.Equal(ambMin, float32(-1))
	})
}

// Test for calcCPUUsage
func TestCalcCPUUsage(t *testing.T) {
	assert := assert.New(t)
	testNodes := []struct {
		pod   []*v1.Pod
		nodes []*v1.Node
	}{
		{
			pod:   []*v1.Pod{makePod("node-test1", "pod-test1", "1000m", "1000m")},
			nodes: []*v1.Node{makeNode("node-test1", 30000, 20000, map[string]string{})},
		},
		{
			pod:   []*v1.Pod{makePod("node-test2", "pod-test2", "5000m", "0m")},
			nodes: []*v1.Node{makeNode("node-test2", 30000, 20000, map[string]string{})},
		},
		{
			pod:   []*v1.Pod{makePod("node-test3", "pod-test3", "0m", "0m")},
			nodes: []*v1.Node{makeNode("node-test3", 30000, 20000, map[string]string{})},
		},
		{
			pod:   []*v1.Pod{makePod("node-test4", "pod-test4", "1000m", "0m")},
			nodes: []*v1.Node{makeNode("node-test4", 30000, 20000, map[string]string{})},
		},
	}

	oco := OsmoticComputingOptimizer{}
	oco.startingPodCPU = make(map[string]float64)

	t.Run("Normal case", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[0].nodes[0])
		nodeInfo.AddPod(testNodes[0].pod[0])
		mockObj := new(mockGetInformations)
		nodeMetrics := &v1beta1.NodeMetrics{
			Usage: v1.ResourceList{
				v1.ResourceCPU: *resource.NewMilliQuantity(
					5000,
					resource.DecimalSI,
				),
			},
		}
		podMetrics := &v1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-metrics",
			},
			Containers: []v1beta1.ContainerMetrics{
				{
					Name: "container",
					Usage: v1.ResourceList{
						v1.ResourceCPU: *resource.NewMilliQuantity(
							15000,
							resource.DecimalSI),
					},
				},
			},
		}

		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, nil)
		mockObj.On("getPodMetrics", mock.Anything).Return(podMetrics, nil)

		oco.getInfo = mockObj
		oco.startingPodCPU[testNodes[0].pod[0].ObjectMeta.Name] = float64(35.5)
		beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(context.Background(), nodeInfo.Node().Name, nodeInfo, testNodes[0].pod[0])

		assert.Nil(calcCPUUsageErr)
		assert.Equal(beforeUsage, float32(0.85))
		assert.Equal(afterUsage, float32(0.8833333))
	})

	t.Run("Normal case [Return usage is -1 because of a lack of resources]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[1].nodes[0])
		nodeInfo.AddPod(testNodes[1].pod[0])
		mockObj := new(mockGetInformations)
		nodeMetrics := &v1beta1.NodeMetrics{
			Usage: v1.ResourceList{
				v1.ResourceCPU: *resource.NewMilliQuantity(
					5000,
					resource.DecimalSI,
				),
			},
		}
		podMetrics := &v1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-metrics",
			},
			Containers: []v1beta1.ContainerMetrics{
				{
					Name: "container",
					Usage: v1.ResourceList{
						v1.ResourceCPU: *resource.NewMilliQuantity(
							15000,
							resource.DecimalSI),
					},
				},
			},
		}

		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, nil)
		mockObj.On("getPodMetrics", mock.Anything).Return(podMetrics, nil)

		oco.getInfo = mockObj
		oco.startingPodCPU[testNodes[1].pod[0].ObjectMeta.Name] = float64(35.5)
		beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(context.Background(), nodeInfo.Node().Name, nodeInfo, testNodes[1].pod[0])

		assert.Nil(calcCPUUsageErr)
		assert.Equal(beforeUsage, float32(-1))
		assert.Equal(afterUsage, float32(-1))
	})

	t.Run("Normal case [Delete startingPodCPU element]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[2].nodes[0])
		nodeInfo.AddPod(testNodes[2].pod[0])
		mockObj := new(mockGetInformations)
		nodeMetrics := &v1beta1.NodeMetrics{
			Usage: v1.ResourceList{
				v1.ResourceCPU: *resource.NewMilliQuantity(
					5000,
					resource.DecimalSI,
				),
			},
		}
		podMetrics := &v1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-metrics",
			},
			Containers: []v1beta1.ContainerMetrics{
				{
					Name: "container",
					Usage: v1.ResourceList{
						v1.ResourceCPU: *resource.NewMilliQuantity(
							16000,
							resource.DecimalSI),
					},
				},
			},
		}

		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, nil)
		mockObj.On("getPodMetrics", mock.Anything).Return(podMetrics, nil)

		oco.getInfo = mockObj
		oco.startingPodCPU[testNodes[2].pod[0].ObjectMeta.Name] = float64(30.5)
		beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(context.Background(), nodeInfo.Node().Name, nodeInfo, testNodes[2].pod[0])

		_, ok := oco.startingPodCPU[testNodes[2].pod[0].ObjectMeta.Name]
		assert.False(ok)
		assert.Nil(calcCPUUsageErr)
		assert.Equal(beforeUsage, float32(-1))
		assert.Equal(afterUsage, float32(-1))
	})

	t.Run("Normal case [Use CPU limits because CPU requests are not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[3].nodes[0])
		nodeInfo.AddPod(testNodes[3].pod[0])
		mockObj := new(mockGetInformations)
		nodeMetrics := &v1beta1.NodeMetrics{
			Usage: v1.ResourceList{
				v1.ResourceCPU: *resource.NewMilliQuantity(
					5000,
					resource.DecimalSI,
				),
			},
		}
		podMetrics := &v1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-metrics",
			},
			Containers: []v1beta1.ContainerMetrics{
				{
					Name: "container",
					Usage: v1.ResourceList{
						v1.ResourceCPU: *resource.NewMilliQuantity(
							15000,
							resource.DecimalSI),
					},
				},
			},
		}

		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, nil)
		mockObj.On("getPodMetrics", mock.Anything).Return(podMetrics, nil)

		oco.getInfo = mockObj
		beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(context.Background(), nodeInfo.Node().Name, nodeInfo, testNodes[3].pod[0])

		assert.Nil(calcCPUUsageErr)
		assert.Equal(beforeUsage, float32(0.16666667))
		assert.Equal(afterUsage, float32(0.195))
	})

	t.Run("Abnormal case [GetNodeMetrics error]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes[0].nodes[0])
		mockObj := new(mockGetInformations)
		nodeMetrics := &v1beta1.NodeMetrics{}
		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, errors.New("error1"))

		oco.getInfo = mockObj
		beforeUsage, afterUsage, calcCPUUsageErr := oco.calcCPUUsage(context.Background(), nodeInfo.Node().Name, nodeInfo, testNodes[0].pod[0])

		assert.Equal(calcCPUUsageErr.Reasons()[0], "Metrics of node "+testNodes[0].nodes[0].Name+" cannot be got")
		assert.Equal(beforeUsage, float32(-1))
		assert.Equal(afterUsage, float32(-1))
	})
}

// Test for calcNormalizeTemperature
func TestCalcNormalizeTemperature(t *testing.T) {
	assert := assert.New(t)
	var oco OsmoticComputingOptimizer
	var h framework.Handle
	oco.handle = h
	oco.powerConsumptionCache = make(map[cacheKey]float32)
	oco.ambient = make(map[string]float32)
	oco.cpu1 = make(map[string]float32)
	oco.cpu2 = make(map[string]float32)
	oco.ambientTimestamp = make(map[string]time.Time)
	oco.startingPodCPU = make(map[string]float64)

	testNodes := struct {
		node []*v1.Node
	}{
		node: []*v1.Node{
			makeNode("node-test1", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5", "cpu1/max": "42", "cpu1/min": "24", "cpu2/max": "49", "cpu2/min": "23"}),
			makeNode("node-test2", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5", "cpu1/max": "29", "cpu1/min": "24", "cpu2/max": "38", "cpu2/min": "23"}),
			makeNode("node-test3", 30000, 20000, map[string]string{}),
			makeNode("node-test4", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5", "cpu2/max": "49", "cpu2/min": "23"}),
			makeNode("node-test5", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5", "cpu1/max": "42", "cpu1/min": "24"}),
		},
	}

	testNodes.node[0].Status.Addresses = append(testNodes.node[0].Status.Addresses, v1.NodeAddress{
		Type:    v1.NodeInternalIP,
		Address: "99.99.99.99",
	})

	oco.ambientTimestamp = make(map[string]time.Time)
	t.Run("Normal case [Get information from GetNodeTemperature]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		testData := []*p2j.Family{
			{
				Name: "ipmi_temperature_celsius",
				Metrics: []interface{}{
					p2j.Metric{
						Labels:      map[string]string{"id": "1", "name": "Ambient"},
						TimestampMs: "",
						Value:       "24",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "10", "name": "CPU2"},
						TimestampMs: "",
						Value:       "36",
					},
					p2j.Metric{
						Labels:      map[string]string{"id": "9", "name": "CPU1"},
						TimestampMs: "",
						Value:       "33",
					},
				},
			},
		}
		mockObj.On("getFamilyInfo", mock.Anything).Return(testData)

		oco.getInfo = mockObj
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.Nil(err)
		assert.Equal(amb, float32(0.5))
		assert.Equal(cpu1, float32(0.5))
		assert.Equal(cpu2, float32(0.5))
	})

	oco.ambientTimestamp = make(map[string]time.Time)
	oco.ambient[testNodes.node[0].Name] = 20.5
	oco.cpu1[testNodes.node[0].Name] = 28.5
	oco.cpu2[testNodes.node[0].Name] = 29.5
	t.Run("Normal case [GetNodeTemperature error, but use OsmoticComputingOptimizer information]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		mockObj.On("getFamilyInfo", mock.Anything).Return([]*p2j.Family{})

		oco.getInfo = mockObj
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.Nil(err)
		assert.Equal(amb, float32(0.36))
		assert.Equal(cpu1, float32(0.25))
		assert.Equal(cpu2, float32(0.25))
	})

	oco.ambient[testNodes.node[0].Name] = 22.5
	oco.cpu1[testNodes.node[0].Name] = 30.0
	oco.cpu2[testNodes.node[0].Name] = 32.5
	oco.ambientTimestamp[testNodes.node[0].Name] = time.Now()
	t.Run("Normal case [Use OsmoticComputingOptimizer information]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.Nil(err)
		assert.Equal(amb, float32(0.44))
		assert.Equal(cpu1, float32(0.33333334))
		assert.Equal(cpu2, float32(0.3653846))
	})

	oco.ambientTimestamp = make(map[string]time.Time)
	oco.ambient[testNodes.node[1].Name] = 18.5
	oco.cpu1[testNodes.node[1].Name] = 26.5
	oco.cpu2[testNodes.node[1].Name] = 30.5
	t.Run("Normal case [Cannot get node internal IP]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[1])

		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.Nil(err)
		assert.Equal(amb, float32(0.28))
		assert.Equal(cpu1, float32(0.5))
		assert.Equal(cpu2, float32(0.5))
	})

	oco.ambient = make(map[string]float32)
	oco.cpu1 = make(map[string]float32)
	oco.cpu2 = make(map[string]float32)
	oco.ambientTimestamp = make(map[string]time.Time)
	t.Run("Abnormal case [GetNodeTemperature error and OsmoticComputingOptimizer information is nil]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		mockObj.On("getFamilyInfo", mock.Anything).Return([]*p2j.Family{})

		oco.getInfo = mockObj
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.EqualError(err, "not exist ipmi data")
		assert.Equal(amb, float32(-1))
		assert.Equal(cpu1, float32(-1))
		assert.Equal(cpu2, float32(-1))
	})

	oco.ambientTimestamp[testNodes.node[0].Name] = time.Now()
	t.Run("Abnormal case [Try to use information from OsmoticComputingOptimizer, but it is nil", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.EqualError(err, "not exist ipmi data")
		assert.Equal(amb, float32(-1))
		assert.Equal(cpu1, float32(-1))
		assert.Equal(cpu2, float32(-1))
	})

	t.Run("Abnormal case [ambient/max and ambient/min labels are not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[2])
		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.EqualError(err, "Standard temperature informations label is not defined")
		assert.Equal(amb, float32(-1))
		assert.Equal(cpu1, float32(-1))
		assert.Equal(cpu2, float32(-1))
	})

	t.Run("Abnormal case [cpu1/max and cpu1/min labels are not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[3])
		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.EqualError(err, "Standard temperature informations label is not defined")
		assert.Equal(amb, float32(-1))
		assert.Equal(cpu1, float32(-1))
		assert.Equal(cpu2, float32(-1))
	})

	t.Run("Abnormal case [cpu2/max and cpu2/min labels are not defined]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[4])
		oco.getInfo = getInfomationsImpl{}
		amb, cpu1, cpu2, err := oco.calcNormalizeTemperature(nodeInfo.Node().Name, nodeInfo)

		assert.EqualError(err, "Standard temperature informations label is not defined")
		assert.Equal(amb, float32(-1))
		assert.Equal(cpu1, float32(-1))
		assert.Equal(cpu2, float32(-1))
	})
}

// Test for calcScore
func TestCalcScore(t *testing.T) {
	assert := assert.New(t)
	var oco OsmoticComputingOptimizer
	var h framework.Handle
	oco.handle = h
	oco.powerConsumptionCache = make(map[cacheKey]float32)
	oco.ambient = make(map[string]float32)
	oco.cpu1 = make(map[string]float32)
	oco.cpu2 = make(map[string]float32)
	oco.ambientTimestamp = make(map[string]time.Time)
	oco.startingPodCPU = make(map[string]float64)

	testNodes := struct {
		node []*v1.Node
	}{
		node: []*v1.Node{makeNode("node-1", 30000, 20000, map[string]string{})},
	}

	t.Run("Normal case [predictPC is success]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		mockObj.On("predictPC", mock.Anything).Return(float32(106.89), nil).Times(1)
		mockObj.On("predictPC", mock.Anything).Return(float32(114.62), nil).Times(1)

		oco.getInfo = mockObj
		score := oco.calcScore("node-1", predictInput{0.1, 0.4, 0.8, 1.1}, predictInput{0.2, 0.4, 0.8, 1.1}, nodeInfo)
		_, ok := oco.getPCCache(cacheKey{"node-1", predictInput{0.1, 0.4, 0.8, 1.1}})
		assert.Equal(score, int64(7))
		assert.Equal(ok, true)
	})

	t.Run("Normal case [Use consumption power from the cache]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		oco.getInfo = getInfomationsImpl{}
		score := oco.calcScore("node-1", predictInput{0.1, 0.4, 0.8, 1.1}, predictInput{0.2, 0.4, 0.8, 1.1}, nodeInfo)

		assert.Equal(score, int64(7))
	})

	t.Run("Abnormal case [predictPC is error(infoBefore)]", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		mockObj.On("predictPC", mock.Anything).Return(float32(-1), errors.New("error"))

		oco.getInfo = mockObj
		score := oco.calcScore("node-1", predictInput{0.4, 0.4, 0.8, 1.1}, predictInput{0.9, 0.9, 0.9, 1.9}, nodeInfo)
		_, ok := oco.getPCCache(cacheKey{"node-1", predictInput{0.4, 0.4, 0.8, 1.1}})

		assert.Equal(score, int64(-1))
		assert.Equal(ok, false)
	})

	t.Run("Abnormal case [predictPC is error(infoAfter)", func(t *testing.T) {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(testNodes.node[0])
		mockObj := new(mockGetInformations)
		mockObj.On("predictPC", mock.Anything).Return(float32(10), nil).Times(1)
		mockObj.On("predictPC", mock.Anything).Return(float32(-1), errors.New("error")).Times(1)

		oco.getInfo = mockObj
		score := oco.calcScore("node-1", predictInput{0.1, 0.1, 0.1, 0.1}, predictInput{0.9, 0.9, 0.9, 1.9}, nodeInfo)
		_, ok1 := oco.getPCCache(cacheKey{"node-1", predictInput{0.1, 0.1, 0.1, 0.1}})
		_, ok2 := oco.getPCCache(cacheKey{"node-1", predictInput{0.9, 0.9, 0.9, 1.9}})

		assert.Equal(score, int64(-1))
		assert.Equal(ok1, true)
		assert.Equal(ok2, false)
	})
}

// Test for Score
// func TestScore(t *testing.T) {
// 	assert := assert.New(t)
// 	state := framework.NewCycleState()
// 	testNodes := []struct {
// 		pod   []*v1.Pod
// 		nodes []*v1.Node
// 	}{
// 		{
// 			pod:   []*v1.Pod{makePod("node-test1", "pod-test1", "2000m", "2000m")},
// 			nodes: []*v1.Node{makeNode("node-test1", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5", "cpu1/max": "42", "cpu1/min": "24", "cpu2/max": "49", "cpu2/min": "23"})},
// 		},
// 		{
// 			pod:   []*v1.Pod{makePod("node-test2", "pod-test2", "0m", "0m")},
// 			nodes: []*v1.Node{makeNode("node-test2", 30000, 20000, map[string]string{"ambient/max": "36.5", "ambient/min": "11.5"})},
// 		},
// 		{
// 			pod:   []*v1.Pod{makePod("node-test3", "pod-test3", "1000m", "1000m")},
// 			nodes: []*v1.Node{makeNode("node-test3", 30000, 20000, map[string]string{})},
// 		},
// 		{
// 			pod:   []*v1.Pod{makePod("node-test4", "pod-test3", "0m", "0m")},
// 			nodes: []*v1.Node{},
// 		},
// 	}
// 	testNodeMetrics := &v1beta1.NodeMetrics{
// 		Usage: v1.ResourceList{
// 			v1.ResourceCPU: *resource.NewMilliQuantity(
// 				7000,
// 				resource.DecimalSI,
// 			),
// 		},
// 	}
// 	testPodMetrics := &v1beta1.PodMetrics{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: "pod-metrics",
// 		},
// 		Containers: []v1beta1.ContainerMetrics{
// 			{
// 				Name: "container",
// 				Usage: v1.ResourceList{
// 					v1.ResourceCPU: *resource.NewMilliQuantity(
// 						15000,
// 						resource.DecimalSI),
// 				},
// 			},
// 		},
// 	}
// 	testFamilyData := []*p2j.Family{
// 		{
// 			Name: "ipmi_temperature_celsius",
// 			Metrics: []interface{}{
// 				p2j.Metric{
// 					Labels:      map[string]string{"id": "1", "name": "Ambient"},
// 					TimestampMs: "",
// 					Value:       "24",
// 				},
// 				p2j.Metric{
// 					Labels:      map[string]string{"id": "10", "name": "CPU2"},
// 					TimestampMs: "",
// 					Value:       "36",
// 				},
// 				p2j.Metric{
// 					Labels:      map[string]string{"id": "9", "name": "CPU1"},
// 					TimestampMs: "",
// 					Value:       "33",
// 				},
// 			},
// 		},
// 	}

// 	testNodes[0].nodes[0].Status.Addresses = append(testNodes[0].nodes[0].Status.Addresses, v1.NodeAddress{
// 		Type:    v1.NodeInternalIP,
// 		Address: "22.22.22.22",
// 	})

// 	oco := OsmoticComputingOptimizer{}
// 	oco.powerConsumptionCache = make(map[cacheKey]float32)
// 	oco.ambient = make(map[string]float32)
// 	oco.cpu1 = make(map[string]float32)
// 	oco.cpu2 = make(map[string]float32)
// 	oco.ambientTimestamp = make(map[string]time.Time)
// 	oco.startingPodCPU = make(map[string]float64)

// 	t.Run("Normal case", func(t *testing.T) {
// 		fh, _ := runtime.NewFramework(nil, nil, nil, runtime.WithSnapshotSharedLister(cache.NewSnapshot(nil, testNodes[0].nodes)))
// 		oco.handle = fh
// 		mockObj := new(mockGetInformations)
// 		mockObj.On("getNodeMetrics", mock.Anything).Return(testNodeMetrics, nil)
// 		mockObj.On("getPodMetrics", mock.Anything).Return(testPodMetrics, nil)
// 		mockObj.On("getFamilyInfo", mock.Anything).Return(testFamilyData)
// 		mockObj.On("predictPC", mock.Anything).Return(float32(106.89), nil).Times(1)
// 		mockObj.On("predictPC", mock.Anything).Return(float32(114.62), nil).Times(1)
// 		oco.getInfo = mockObj
// 		oco.startingPodCPU[testNodes[0].pod[0].ObjectMeta.Name] = float64(35.5)
// 		result, err := oco.Score(context.Background(), state, testNodes[0].pod[0], testNodes[0].nodes[0].Name)

// 		assert.Equal(result, int64(7))
// 		assert.Nil(err)
// 	})

// 	t.Run("Abnormal case [Can't get NodeInfo]", func(t *testing.T) {
// 		fh, _ := runtime.NewFramework(nil, nil, nil, runtime.WithSnapshotSharedLister(cache.NewSnapshot(nil, testNodes[3].nodes)))
// 		oco.handle = fh
// 		nodeName := "test-node"
// 		result, err := oco.Score(context.Background(), state, testNodes[3].pod[0], nodeName)
// 		assert.Equal(result, int64(-1))
// 		assert.Equal(err, framework.NewStatus(framework.Error, fmt.Sprintf("getting node test-node from Snapshot: nodeinfo not found for node name \"test-node\"")))
// 	})

// 	t.Run("Abnormal case [Can't get NodeMetrics]", func(t *testing.T) {
// 		fh, _ := runtime.NewFramework(nil, nil, nil, runtime.WithSnapshotSharedLister(cache.NewSnapshot(nil, testNodes[0].nodes)))
// 		oco.handle = fh
// 		mockObj := new(mockGetInformations)
// 		nodeMetrics := &v1beta1.NodeMetrics{}
// 		mockObj.On("getNodeMetrics", mock.Anything).Return(nodeMetrics, errors.New("Cannot get NodeMetrics info"))
// 		oco.getInfo = mockObj
// 		result, err := oco.Score(context.Background(), state, testNodes[0].pod[0], testNodes[0].nodes[0].Name)
// 		assert.Equal(result, int64(-1))
// 		assert.Equal(err, framework.NewStatus(framework.Error, fmt.Sprintf("Metrics of node %v cannot be got", testNodes[0].nodes[0].Name)))
// 	})

// 	t.Run("Abnormal case [Pod does not define requested and limited resources]", func(t *testing.T) {
// 		fh, _ := runtime.NewFramework(nil, nil, nil, runtime.WithSnapshotSharedLister(cache.NewSnapshot(nil, testNodes[1].nodes)))
// 		oco.handle = fh
// 		mockObj := new(mockGetInformations)
// 		mockObj.On("getNodeMetrics", mock.Anything).Return(testNodeMetrics, nil)
// 		oco.getInfo = mockObj
// 		result, err := oco.Score(context.Background(), state, testNodes[1].pod[0], testNodes[1].nodes[0].Name)
// 		assert.Equal(result, int64(-1))
// 		assert.Nil(err)
// 	})

// 	t.Run("Abnormal case [ambient/max and ambient/min labels are not defined]", func(t *testing.T) {
// 		fh, _ := runtime.NewFramework(nil, nil, nil, runtime.WithSnapshotSharedLister(cache.NewSnapshot(nil, testNodes[2].nodes)))
// 		oco.handle = fh
// 		mockObj := new(mockGetInformations)
// 		mockObj.On("getNodeMetrics", mock.Anything).Return(testNodeMetrics, nil)
// 		oco.getInfo = mockObj
// 		result, err := oco.Score(context.Background(), state, testNodes[2].pod[0], testNodes[2].nodes[0].Name)
// 		assert.Equal(result, int64(-1))
// 		assert.Nil(err)
// 	})
// }

// Test for NormalizeScore
func TestNormalizeScore(t *testing.T) {
	assert := assert.New(t)
	state := framework.NewCycleState()
	var oco OsmoticComputingOptimizer
	oco.startingPodCPU = make(map[string]float64)
	testNodes := []struct {
		pod []*v1.Pod
	}{
		{
			pod: []*v1.Pod{makePod("node-test1", "pod-test1", "500m", "0m")},
		},
		{
			pod: []*v1.Pod{makePod("node-test2", "pod-test2", "0m", "500m")},
		},
	}
	t.Run("Normal case [The smaller the score, the larger the normalized score]", func(t *testing.T) {
		scoreList := []framework.NodeScore{
			{
				Name:  "node-0",
				Score: -1,
			},
			{
				Name:  "node-1",
				Score: 1,
			},
			{
				Name:  "node-2",
				Score: 4,
			},
			{
				Name:  "node-3",
				Score: 5,
			},
			{
				Name:  "node-4",
				Score: 7,
			},
			{
				Name:  "node-5",
				Score: 9,
			},
		}

		var testData framework.NodeScoreList
		testData = scoreList

		oco.NormalizeScore(context.Background(), state, testNodes[0].pod[0], testData)
		assert.Equal(testData[0].Score, int64(0))
		assert.Equal(testData[1].Score, int64(100))
		assert.Equal(testData[2].Score, int64(63))
		assert.Equal(testData[3].Score, int64(50))
		assert.Equal(testData[4].Score, int64(25))
		assert.Equal(testData[5].Score, int64(0))
		assert.Equal(oco.startingPodCPU[testNodes[0].pod[0].Name], float64(0.425))
	})

	t.Run("Normal case [If the maximum and minimum scores are the same, the normalized scores are all tempScore]", func(t *testing.T) {
		scoreList := []framework.NodeScore{
			{
				Name:  "node-0",
				Score: 5,
			},
			{
				Name:  "node-1",
				Score: -1,
			},
			{
				Name:  "node-2",
				Score: 5,
			},
		}

		var testData framework.NodeScoreList
		testData = scoreList

		oco.NormalizeScore(context.Background(), state, testNodes[1].pod[0], testData)
		assert.Equal(testData[0].Score, int64(tempScore))
		assert.Equal(testData[1].Score, int64(0))
		assert.Equal(testData[2].Score, int64(tempScore))
		assert.Equal(oco.startingPodCPU[testNodes[1].pod[0].Name], float64(0.5))
	})

	t.Run("Normal case [If the score is negative, the normalized score is 0]", func(t *testing.T) {
		scoreList := []framework.NodeScore{
			{
				Name:  "node-0",
				Score: -1,
			},
			{
				Name:  "node-1",
				Score: -1,
			},
			{
				Name:  "node-2",
				Score: -1,
			},
		}

		var testData framework.NodeScoreList
		testData = scoreList

		oco.NormalizeScore(context.Background(), state, testNodes[1].pod[0], testData)
		assert.Equal(testData[0].Score, int64(0))
		assert.Equal(testData[1].Score, int64(0))
		assert.Equal(testData[2].Score, int64(0))
	})
}

// Test for New
func TestNew(t *testing.T) {
	assert := assert.New(t)
	t.Run("Normal case", func(t *testing.T) {
		var runtime pkgruntime.Object
		var h framework.Handle
		result, err := New(runtime, h)
		assert.Equal(result.Name(), MinimizePowerName)
		assert.Equal(result.(*OsmoticComputingOptimizer).handle, h)
		assert.Equal(result.(*OsmoticComputingOptimizer).ambient, map[string]float32{})
		assert.Equal(result.(*OsmoticComputingOptimizer).cpu1, map[string]float32{})
		assert.Equal(result.(*OsmoticComputingOptimizer).cpu2, map[string]float32{})
		assert.Equal(result.(*OsmoticComputingOptimizer).ambientTimestamp, map[string]time.Time{})
		assert.Equal(result.(*OsmoticComputingOptimizer).powerConsumptionCache, map[cacheKey]float32{})
		assert.Equal(result.(*OsmoticComputingOptimizer).startingPodCPU, map[string]float64{})
		assert.Equal(result.(*OsmoticComputingOptimizer).getInfo, getInfomationsImpl{})
		assert.Nil(err)
	})
}
