package podspread

import (
	"context"
	"errors"
	"fmt"
	"math"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"
)

type PodSpread struct {
	schedulingSession map[string]*SchedulingSession
	k8sClient         *kubernetes.Clientset
}

type SchedulingSession struct {
	TotalReplicas    int
	Redunduncy       int
	DeployedNodes    map[string]int
	TotalDeployed    int
	SpreadMode       SpreadMode
	SpreadInfoRegion SpreadInfo
	SpreadInfoZone   SpreadInfo
}

type SpreadInfo map[string]map[string]bool

func (s SpreadInfo) GetNodeArea(node string) (string, bool) {
	for area, nodes := range s {
		_, ok := nodes[node]
		if ok {
			return area, true
		}
	}
	return "", false
}

func (s SpreadInfo) GetAreasOnlyControlPlane() []string {
	var a []string
	for area, nodes := range s {
		onlyCP := true
		for _, isCP := range nodes {
			if !isCP {
				onlyCP = false
				break
			}
		}
		if onlyCP {
			a = append(a, area)
		}
	}
	return a
}

type SpreadMode string

const (
	SpreadModeRegion SpreadMode = "SpreadModeRegion"
	SpreadModeZone   SpreadMode = "SpreadModeZone"
	SpreadModeNode   SpreadMode = "SpreadModeNode"
)

var _ framework.FilterPlugin = &PodSpread{}

var (
	Name = "PodSpread"

	ErrReasonNodeNotFound = errors.New("node not found")
	// ErrReasonPodAlreadyDeployed = errors.New("one or more pods already deployed to the node")
	// ErrReasonIsBetterNode       = errors.New("there is a better node to deploy the pod")
)

// Name returns name of the plugin. It is used in logs, etc.
func (*PodSpread) Name() string {
	return Name
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	// initialize a K8s client
	klog.V(1).InfoS("Test: 0: initializing client-go")

	// NOTE: kube-scheduler does not have in cluster config
	// config, err := rest.InClusterConfig()
	config, err := clientcmd.BuildConfigFromFlags("", "/etc/kubernetes/scheduler.conf")
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	klog.V(1).InfoS("Test: 0: OK")

	pl := &PodSpread{
		schedulingSession: map[string]*SchedulingSession{},
		k8sClient:         clientset,
	}
	return pl, nil
}

// Check the node name and deploy pods to the schedulable node.
func (pl *PodSpread) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {

	klog.V(1).InfoS("Filter: PodSpread", "pod", pod.Name, "node", nodeInfo.Node().Name)

	if nodeInfo.Node().Name == "" {
		fmt.Println("node name not found")
		return framework.NewStatus(framework.Error, ErrReasonNodeNotFound.Error())
	}

	// PodのReplicaSetを取得
	replicaSetName := "" // TODO
	for _, ref := range pod.OwnerReferences {
		if pointer.BoolDeref(ref.Controller, false) && ref.Kind == "ReplicaSet" {
			replicaSetName = ref.Name
		}
	}
	if replicaSetName == "" {
		// no controller found
		return nil
	}
	rs, err := pl.k8sClient.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, replicaSetName, metav1.GetOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// PodListを取得
	podList, err := pl.k8sClient.CoreV1().Pods(rs.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// NodeListを取得
	nodeList, err := pl.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// schedulingSessionの更新
	pl.updateSchedulingSession(ctx, rs, podList, nodeList)

	// 配置できないノードをリストする
	filteredNodes, err := getUnallocatableNodes(pl.schedulingSession[rs.Name], pod, nodeList)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	klog.V(1).InfoS("list unallocatable node", "filteredNodes", filteredNodes)

	// Filter処理
	for _, n := range filteredNodes {
		if nodeInfo.Node().Name == n {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "unschedulable reason:filterd node")
		}
	}
	return nil
}

const (
	// https://kubernetes.io/docs/reference/labels-annotations-taints/
	labelZone         = "topology.kubernetes.io/zone"
	labelRegion       = "topology.kubernetes.io/region"
	labelControlPlane = "node-role.kubernetes.io/control-plane"
)

func getSpreadMode(nodeList *corev1.NodeList) (mode SpreadMode, regions, zones SpreadInfo) {
	zones = SpreadInfo{} // key: [zone][node] value: isMasterNode?
	for _, n := range nodeList.Items {
		_, isCP := n.Labels[labelControlPlane]
		zone, ok := n.Labels[labelZone]
		if !ok {
			continue
		}
		if _, ok = zones[zone]; !ok {
			zones[zone] = map[string]bool{}
		}
		zones[zone][n.Name] = isCP
	}

	regions = SpreadInfo{} // key: [region][node] value: isMasterNode?
	for _, n := range nodeList.Items {
		_, isCP := n.Labels[labelControlPlane]
		region, ok := n.Labels[labelRegion]
		if !ok {
			continue
		}
		if _, ok = regions[region]; !ok {
			regions[region] = map[string]bool{}
		}
		regions[region][n.Name] = isCP
	}

	switch {
	case len(regions) >= 2:
		mode = SpreadModeRegion
	case len(zones) >= 2:
		mode = SpreadModeZone
	default:
		mode = SpreadModeNode
	}
	return
}

func calcRedunduncy(...any) int {
	return 4
}

func (pl *PodSpread) updateSchedulingSession(ctx context.Context, rs *appsv1.ReplicaSet, podList *corev1.PodList, nodeList *corev1.NodeList) error {

	// スケジュール対象Podが所属するReplicaSetのschedulingSessionがなければ初期化
	if _, ok := pl.schedulingSession[rs.Name]; !ok {
		totalReplicas := int(pointer.Int32Deref(rs.Spec.Replicas, 0))
		const p = 0.1                                  // TODO annotationからpをとってくる
		redunduncy := calcRedunduncy(totalReplicas, p) // TODO 計算する

		pl.schedulingSession[rs.Name] = &SchedulingSession{
			TotalReplicas: totalReplicas,
			Redunduncy:    redunduncy,
		}
	}

	// schedulingSessionを更新する
	ss := pl.schedulingSession[rs.Name]

	klog.V(1).InfoS("get scheduling session", "ss", ss)

	// DeployedInfoを更新する
	ss.DeployedNodes = map[string]int{}
	for _, n := range nodeList.Items {
		if _, ok := ss.DeployedNodes[n.Name]; !ok {
			ss.DeployedNodes[n.Name] = 0
		}
	}
	ss.setDeployNodes(rs, podList)

	// SpreadModeを更新する
	mode, regions, zones := getSpreadMode(nodeList)
	ss.SpreadMode = mode
	ss.SpreadInfoRegion = regions
	ss.SpreadInfoZone = zones

	return nil
}

func (ss *SchedulingSession) setDeployNodes(rs *appsv1.ReplicaSet, podList *corev1.PodList) {
	ss.TotalDeployed = 0

	// Pod Conditions に Type:PodScheduled,Status:True があれば、Podの配置先が確定している
	// なぜPodScheduledなのか？ https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
	for _, pod := range podList.Items {
		if !metav1.IsControlledBy(&pod, rs) {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
				ss.DeployedNodes[pod.Spec.NodeName]++
				ss.TotalDeployed++
			}
		}
	}
}

func getUnallocatableNodes(ss *SchedulingSession, pod *corev1.Pod, nodeList *corev1.NodeList) ([]string, error) {

	isControlPlane := func(name string) bool {
		for _, n := range nodeList.Items {
			if n.Name != name {
				continue
			}
			if _, ok := n.Labels[labelControlPlane]; ok {
				return true
			}
		}
		return false
	}

	var denyNodes []string

	// 十分に冗長配置されたので、どのノードにも配置できます
	if ss.TotalDeployed >= ss.Redunduncy {

		klog.V(1).InfoS("Reduduncy is enough")

		return denyNodes, nil
	}

	// Podをマスターノードに配置するための設定
	// https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	//
	// マスターノード限定
	// nodeSelector:
	// 	 node-role.kubernetes.io/control-plane: ""
	//
	// マスターノードもOK
	// tolerations:
	// 	 - key: "node-role.kubernetes.io/control-plane"
	// 	   operator: "Exists"
	// 	   effect: "NoSchedule"
	isControlPlaneSchedulable := false
	for _, t := range pod.Spec.Tolerations {
		if t.Key == labelControlPlane && t.Operator == corev1.TolerationOpExists && t.Effect == corev1.TaintEffectNoSchedule {
			isControlPlaneSchedulable = true
		}
	}

	switch ss.SpreadMode {
	case SpreadModeRegion:
		regionDeployed := map[string]int{}
		var minRegion []string
		skip := false

		// 各regionの総配置数取得
		// cp1つしかないregionは配置対象外
		for region := range ss.SpreadInfoRegion {
			deployed := 0
			for node, cp := range ss.SpreadInfoRegion[region] {
				if cp && len(ss.SpreadInfoRegion[region]) == 1 {
					skip = true
				}
				deployed += ss.DeployedNodes[node]
			}
			regionDeployed[region] = deployed
		}

		// 全regionの最小配置数決定
		minNum := math.MaxInt
		for _, num := range regionDeployed {
			if num < minNum {
				minNum = num
			}
		}

		// 最小配置数のregion配列取得
		for region, deployed := range regionDeployed {
			if skip {
				continue
			}
			if deployed == minNum {
				minRegion = append(minRegion, region)
			}
		}

		allowNodes := map[string]struct{}{}

		for _, region := range minRegion {
			for node, cp := range ss.SpreadInfoRegion[region] {
				if cp && !isControlPlaneSchedulable {
					continue
				}
				allowNodes[node] = struct{}{}
			}
		}

		for _, node := range nodeList.Items {
			if _, ok := allowNodes[node.Name]; ok {
				continue
			}
			denyNodes = append(denyNodes, node.Name)
		}

		return denyNodes, nil
	case SpreadModeZone:
		zoneDeployed := map[string]int{}
		var minZone []string
		skip := false

		// 各zoneの総配置数取得
		for zone := range ss.SpreadInfoZone {
			deployed := 0
			for node, cp := range ss.SpreadInfoZone[zone] {
				if cp && len(ss.SpreadInfoZone[zone]) == 1 {
					skip = true
				}
				deployed += ss.DeployedNodes[node]
			}
			zoneDeployed[zone] = deployed
		}

		// 全zoneの最小配置数決定
		minNum := math.MaxInt
		for _, num := range zoneDeployed {
			if num < minNum {
				minNum = num
			}
		}

		// 最小配置数のzone配列取得
		for zone, deployed := range zoneDeployed {
			if skip {
				continue
			}
			if deployed == minNum {
				minZone = append(minZone, zone)
			}
		}

		allowNodes := map[string]struct{}{}

		for _, zone := range minZone {
			for node, cp := range ss.SpreadInfoZone[zone] {
				if cp && !isControlPlaneSchedulable {
					continue
				}
				allowNodes[node] = struct{}{}
			}
		}

		for _, node := range nodeList.Items {
			if _, ok := allowNodes[node.Name]; ok {
				continue
			}
			denyNodes = append(denyNodes, node.Name)
		}

		return denyNodes, nil
	case SpreadModeNode:
		allowNodes := map[string]struct{}{}
		minDeployed := math.MaxInt

		for node, deployed := range ss.DeployedNodes {
			if isControlPlane(node) && !isControlPlaneSchedulable {
				continue
			}
			// klog.V(1).InfoS("make node map", "node", node, "deployed", deployed)
			if deployed < minDeployed {
				minDeployed = deployed
			}
		}

		for node, deployed := range ss.DeployedNodes {
			if isControlPlane(node) && !isControlPlaneSchedulable {
				continue
			}
			if deployed == minDeployed {
				allowNodes[node] = struct{}{}
			}
		}

		// klog.V(1).InfoS("decide allowNodes", "allowNodes", allowNodes)

		for _, node := range nodeList.Items {
			if _, ok := allowNodes[node.Name]; ok {
				continue
			}
			denyNodes = append(denyNodes, node.Name)
		}

		// klog.V(1).InfoS("decide denyNodes", "denyNodes", denyNodes)

		return denyNodes, nil
	default:
		return nil, fmt.Errorf("invalid SpreadMode :%v", ss.SpreadMode)
	}
}
