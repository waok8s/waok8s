package podspread

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var (
	testSI1 = SpreadInfo{
		"z0": map[string]bool{
			"n00": true, "n01": false, "n02": false,
		},
		"z1": map[string]bool{
			"n10": true, "n11": false, "n12": false,
		},
		"z2": map[string]bool{
			"n20": true, "n21": false, "n22": false,
		},
	}
)

func TestSpreadInfo_GetNodeArea(t *testing.T) {
	type args struct {
		node string
	}
	tests := []struct {
		name  string
		s     SpreadInfo
		args  args
		want  string
		want1 bool
	}{
		{"z0", testSI1, args{"n01"}, "z0", true},
		{"z1", testSI1, args{"n10"}, "z1", true},
		{"z2", testSI1, args{"n22"}, "z2", true},
		{"f", testSI1, args{"n123"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.s.GetNodeArea(tt.args.node)
			if got != tt.want {
				t.Errorf("SpreadInfo.GetNodeArea() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("SpreadInfo.GetNodeArea() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestSpreadInfo_GetAreasOnlyControlPlane(t *testing.T) {
	tests := []struct {
		name string
		s    SpreadInfo
		want []string
	}{
		{"0", SpreadInfo{
			"z0": map[string]bool{
				"n00": true, "n01": false, "n02": false,
			},
			"z1": map[string]bool{
				"n10": true, "n11": false, "n12": false,
			},
			"z2": map[string]bool{
				"n20": true, "n21": false, "n22": false,
			},
		}, []string{}},
		{"1", SpreadInfo{
			"z0": map[string]bool{
				"n00": true, "n01": false, "n02": false,
			},
			"z1": map[string]bool{
				"n10": true,
			},
			"z2": map[string]bool{
				"n20": true, "n21": false, "n22": false,
			},
		}, []string{"z1"}},
		{"2", SpreadInfo{
			"z0": map[string]bool{
				"n00": true, "n01": false, "n02": false,
			},
			"z1": map[string]bool{
				"n10": true,
			},
			"z2": map[string]bool{
				"n20": true,
			},
		}, []string{"z1", "z2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.GetAreasOnlyControlPlane()
			gotM := map[string]struct{}{}
			for _, v := range got {
				gotM[v] = struct{}{}
			}
			for _, want := range tt.want {
				_, ok := gotM[want]
				if !ok {
					t.Errorf("SpreadInfo.GetAreaOnlyControlPlane() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

var (
	testNS  = "default"
	testRS1 = &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: testNS,
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: pointer.Int32Ptr(10),
		},
	}
	ownedByRS1 = metav1.OwnerReference{
		APIVersion:         testRS1.APIVersion,
		Kind:               testRS1.Kind,
		Name:               testRS1.Name,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	testPod1 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p1",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod2 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p2",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod3 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p3",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod4 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p4",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod5 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p5",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod6 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p6",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod7 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p7",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod8 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p8",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod9 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p9",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
	testPod10 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p10",
			Namespace:       testNS,
			OwnerReferences: []metav1.OwnerReference{ownedByRS1},
		},
	}
)

func setNode(pod *corev1.Pod, node string) *corev1.Pod {
	p2 := pod.DeepCopy()
	p2.Spec.NodeName = node
	return p2
}

func setConds(pod *corev1.Pod, conds []corev1.PodCondition) *corev1.Pod {
	p2 := pod.DeepCopy()
	p2.Status.Conditions = conds
	return p2
}

var (
	condSchedT = corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}
	condSchedF = corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionFalse}
	condReadyT = corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse}
	condReadyF = corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse}
)

func TestSchedulingSession_setDeployNodes(t *testing.T) {
	type args struct {
		rs      *appsv1.ReplicaSet
		podList *corev1.PodList
	}
	tests := []struct {
		name string
		ss   *SchedulingSession
		args args
		want *SchedulingSession
	}{
		{"0sched", &SchedulingSession{
			DeployedNodes: map[string]int{
				"n00": 0,
				"n01": 0,
				"n02": 0,
			},
			TotalDeployed: 0,
		}, args{testRS1, &corev1.PodList{Items: []corev1.Pod{
			*setConds(setNode(testPod1, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod2, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod3, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod4, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod5, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod6, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod7, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod8, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod9, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod10, ""), []corev1.PodCondition{}),
		}}}, &SchedulingSession{
			DeployedNodes: map[string]int{
				"n00": 0,
				"n01": 0,
				"n02": 0,
			},
			TotalDeployed: 0,
		}},
		{"6sched", &SchedulingSession{
			DeployedNodes: map[string]int{
				"n00": 0,
				"n01": 0,
				"n02": 0,
			},
			TotalDeployed: 0,
		}, args{testRS1, &corev1.PodList{Items: []corev1.Pod{
			*setConds(setNode(testPod1, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod2, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod3, "n00"), []corev1.PodCondition{condSchedT}),
			*setConds(setNode(testPod4, "n01"), []corev1.PodCondition{condSchedT, condSchedF}),
			*setConds(setNode(testPod5, "n02"), []corev1.PodCondition{condSchedT, condSchedF}),
			*setConds(setNode(testPod6, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod7, ""), []corev1.PodCondition{}),
			*setConds(setNode(testPod8, "n00"), []corev1.PodCondition{condSchedT}),
			*setConds(setNode(testPod9, "n01"), []corev1.PodCondition{condReadyF, condSchedT, condSchedF}),
			*setConds(setNode(testPod10, "n02"), []corev1.PodCondition{condReadyT, condSchedT, condSchedF}),
		}}}, &SchedulingSession{
			DeployedNodes: map[string]int{
				"n00": 2,
				"n01": 2,
				"n02": 2,
			},
			TotalDeployed: 6,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ss.setDeployNodes(tt.args.rs, tt.args.podList)
			if !reflect.DeepEqual(tt.ss, tt.want) {
				t.Errorf("setDeployNodes() got = %v, want %v", tt.ss, tt.want)
			}
		})
	}
}

func newNode(name, zone, region string, isCP bool) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
	}
	if region != "" {
		node.Labels[labelRegion] = region
	}
	if zone != "" {
		node.Labels[labelZone] = zone
	}
	if isCP {
		node.Labels[labelControlPlane] = ""
	}
	return node
}

func Test_getSpreadMode(t *testing.T) {
	type args struct {
		nodeList *corev1.NodeList
	}
	tests := []struct {
		name        string
		args        args
		wantMode    SpreadMode
		wantRegions SpreadInfo
		wantZones   SpreadInfo
	}{
		{"0zone/0region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "", "", true), newNode("n01", "", "", false), newNode("n02", "", "", false),
		}}}, SpreadModeNode,
			SpreadInfo{},
			SpreadInfo{},
		},
		{"1zone/0region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "z0", "", true), newNode("n01", "z0", "", false), newNode("n02", "z0", "", false),
		}}}, SpreadModeNode,
			SpreadInfo{},
			SpreadInfo{
				"z0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
			},
		},
		{"2zones/0region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "z0", "", true), newNode("n01", "z0", "", false), newNode("n02", "z0", "", false),
			newNode("n10", "z1", "", true), newNode("n11", "z1", "", false), newNode("n12", "z1", "", false),
		}}}, SpreadModeZone,
			SpreadInfo{},
			SpreadInfo{
				"z0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
				"z1": map[string]bool{
					"n10": true, "n11": false, "n12": false,
				},
			},
		},
		{"0zones/1region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "", "r0", true), newNode("n01", "", "r0", false), newNode("n02", "", "r0", false),
		}}}, SpreadModeNode,
			SpreadInfo{
				"r0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
			},
			SpreadInfo{},
		},
		{"0zones/2region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "", "r0", true), newNode("n01", "", "r0", false), newNode("n02", "", "r0", false),
			newNode("n10", "", "r1", true), newNode("n11", "", "r1", false), newNode("n12", "", "r1", false),
		}}}, SpreadModeRegion,
			SpreadInfo{
				"r0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
				"r1": map[string]bool{
					"n10": true, "n11": false, "n12": false,
				},
			},
			SpreadInfo{},
		},
		{"3zones/2region", args{&corev1.NodeList{Items: []corev1.Node{
			newNode("n00", "z0", "r0", true), newNode("n01", "z0", "r0", false), newNode("n02", "z0", "r0", false),
			newNode("n10", "z1", "r1", true), newNode("n11", "z1", "r1", false), newNode("n12", "z1", "r1", false),
			newNode("n20", "z2", "", true), newNode("n21", "z2", "", false), newNode("n22", "z2", "", false),
		}}}, SpreadModeRegion,
			SpreadInfo{
				"r0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
				"r1": map[string]bool{
					"n10": true, "n11": false, "n12": false,
				},
			},
			SpreadInfo{
				"z0": map[string]bool{
					"n00": true, "n01": false, "n02": false,
				},
				"z1": map[string]bool{
					"n10": true, "n11": false, "n12": false,
				},
				"z2": map[string]bool{
					"n20": true, "n21": false, "n22": false,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotRegions, gotZones := getSpreadMode(tt.args.nodeList)
			if !reflect.DeepEqual(gotMode, tt.wantMode) {
				t.Errorf("PodSpread.getSpreadMode() gotMode = %v, want %v", gotMode, tt.wantMode)
			}
			if !reflect.DeepEqual(gotRegions, tt.wantRegions) {
				t.Errorf("PodSpread.getSpreadMode() gotRegions = %v, want %v", gotRegions, tt.wantRegions)
			}
			if !reflect.DeepEqual(gotZones, tt.wantZones) {
				t.Errorf("PodSpread.getSpreadMode() gotZones = %v, want %v", gotZones, tt.wantZones)
			}
		})
	}
}

func Test_getAllocatableNodes(t *testing.T) {
	type args struct {
		ss       *SchedulingSession
		pod      *corev1.Pod
		nodeList *corev1.NodeList
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{"0zone/2region/01-11", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 1,
					"n10": 1,
					"n11": 1,
				},
				TotalDeployed: 3,
				SpreadMode:    SpreadModeRegion,
				SpreadInfoRegion: SpreadInfo{
					"r0": map[string]bool{
						"n00": true, "n01": false,
					},
					"r1": map[string]bool{
						"n10": false, "n11": false,
					},
				},
				SpreadInfoZone: SpreadInfo{},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "", "r0", true), newNode("n01", "", "r0", false), newNode("n10", "", "r1", false), newNode("n11", "", "r1", false),
			}},
		}, []string{"n01"}, false},
		{"0zone/3region/01-01-01", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 1,
					"n10": 0,
					"n11": 1,
					"n20": 0,
					"n21": 1,
				},
				TotalDeployed: 3,
				SpreadMode:    SpreadModeRegion,
				SpreadInfoRegion: SpreadInfo{
					"r0": map[string]bool{
						"n00": true, "n01": false,
					},
					"r1": map[string]bool{
						"n10": false, "n11": false,
					},
					"r2": map[string]bool{
						"n20": false, "n21": false,
					},
				},
				SpreadInfoZone: SpreadInfo{},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "", "r0", true), newNode("n01", "", "r0", false), newNode("n10", "", "r1", false), newNode("n11", "", "r1", false), newNode("n20", "", "r2", false), newNode("n21", "", "r2", false),
			}},
		}, []string{"n01", "n10", "n11", "n20", "n21"}, false},
		{"2zone/0region/012-110", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 1,
					"n02": 2,
					"n10": 1,
					"n11": 1,
					"n12": 0,
				},
				TotalDeployed:    5,
				SpreadMode:       SpreadModeZone,
				SpreadInfoRegion: SpreadInfo{},
				SpreadInfoZone: SpreadInfo{
					"z0": map[string]bool{
						"n00": true, "n01": false, "n02": false,
					},
					"z1": map[string]bool{
						"n10": false, "n11": false, "n12": false,
					},
				},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "z0", "", true), newNode("n01", "z0", "", false), newNode("n02", "z0", "", false), newNode("n10", "z1", "", false), newNode("n11", "z1", "", false), newNode("n12", "z1", "", false),
			}},
		}, []string{"n10", "n11", "n12"}, false},
		{"node_0211", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 2,
					"n02": 1,
					"n03": 1,
				},
				TotalDeployed:    4,
				SpreadMode:       SpreadModeNode,
				SpreadInfoRegion: SpreadInfo{},
				SpreadInfoZone:   SpreadInfo{},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "", "", true), newNode("n01", "", "", false), newNode("n02", "", "", false), newNode("n03", "", "", false),
			}},
		}, []string{"n02", "n03"}, false},
		{"node_0221", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 2,
					"n02": 2,
					"n03": 1,
				},
				TotalDeployed:    5,
				SpreadMode:       SpreadModeNode,
				SpreadInfoRegion: SpreadInfo{},
				SpreadInfoZone:   SpreadInfo{},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "", "", true), newNode("n01", "", "", false), newNode("n02", "", "", false), newNode("n03", "", "", false),
			}},
		}, []string{"n03"}, false},
		{"node_0222", args{
			ss: &SchedulingSession{
				TotalReplicas: 10,
				Redunduncy:    6,
				DeployedNodes: map[string]int{
					"n00": 0,
					"n01": 2,
					"n02": 2,
					"n03": 2,
				},
				TotalDeployed:    6,
				SpreadMode:       SpreadModeNode,
				SpreadInfoRegion: SpreadInfo{},
				SpreadInfoZone:   SpreadInfo{},
			},
			pod: testPod1,
			nodeList: &corev1.NodeList{Items: []corev1.Node{
				newNode("n00", "", "", true), newNode("n01", "", "", false), newNode("n02", "", "", false), newNode("n03", "", "", false),
			}},
		}, []string{"n00", "n01", "n02", "n03"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAllocatableNodes(tt.args.ss, tt.args.pod, tt.args.nodeList)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAllocatableNodes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotM := map[string]struct{}{}
			for _, v := range got {
				gotM[v] = struct{}{}
			}
			for _, want := range tt.want {
				_, ok := gotM[want]
				if !ok {
					t.Errorf("SpreadInfo.GetAreaOnlyControlPlane() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
