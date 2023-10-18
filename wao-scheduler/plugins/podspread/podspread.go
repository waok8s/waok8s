package podspread

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"
)

type PodSpread struct {
	mu                sync.Mutex
	schedulingSession map[string]*SchedulingSession
	clientset         kubernetes.Interface
}

var _ framework.PreFilterPlugin = &PodSpread{}

type SchedulingSession struct {
	TotalReplicas    int
	Redunduncy       int
	DeployedNodes    map[string]int
	TotalDeployed    int
	SpreadMode       SpreadMode
	SpreadInfoRegion SpreadInfo
	SpreadInfoZone   SpreadInfo
}

// SpreadInfo holds area info.
//
//	area1:           # nodes has "area1" label (zone or region)
//	  node11: true   # node11 is a control plane node
//	  node12: false  # node12 is a worker node
//	area2:
//	  node21: false
type SpreadInfo map[string]map[string]bool

func (s SpreadInfo) GetNodeArea(node string) (area string, isControlPlane bool) {
	for area, nodes := range s {
		_, ok := nodes[node]
		if ok {
			return area, true
		}
	}
	return "", false
}

// GetAreasOnlyControlPlane returns areas no worker nodes.
// These areas should be excluded from scheduling.
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

var (
	Name = "PodSpread"

	ReasonEmptyNodeName             = "node not found"
	ReasonNotControlledByReplicaSet = "the pod is not controlled by any ReplicaSet"
	ReasonK8sClient                 = "skip this plugin as k8sClient got error: "
	ReasonShouldBeSpread            = "pod allocation should be spread"
	ReasonSchedulingSession         = "skip this plugin as wrong SchedulingSession status: "
)

// Name returns name of the plugin. It is used in logs, etc.
func (*PodSpread) Name() string {
	return Name
}

const (
	AnnotationPodSpreadRate = "wao.bitmedia.co.jp/podspread-rate"
)

// New initializes a new plugin and returns it.
func New(plArgs runtime.Object, fh framework.Handle) (framework.Plugin, error) {
	// initialize the plugin
	pl := &PodSpread{
		schedulingSession: map[string]*SchedulingSession{},
		clientset:         fh.ClientSet(),
	}

	return pl, nil
}

// PreFilterExtensions do not exist for this plugin.
func (pl *PodSpread) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// PreFilter invoked at the prefilter extension point.
func (pl *PodSpread) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	// PreFilter()でerrorを返すとpodが配置不可となるので特定のケースでフィルタをパスする
	result := &framework.PreFilterResult{
		NodeNames: nil,
	}

	// PodのReplicaSetを取得
	replicaSetName := ""
	for _, ref := range pod.OwnerReferences {
		if pointer.BoolDeref(ref.Controller, false) && ref.Kind == "ReplicaSet" {
			replicaSetName = ref.Name
		}
	}
	if replicaSetName == "" {
		// no controller found
		klog.V(1).InfoS("error of PreFilter():", "ReasonNotControlledByReplicaSet", ReasonNotControlledByReplicaSet)
		return result, nil
	}
	rs, err := pl.clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, replicaSetName, metav1.GetOptions{})
	if err != nil {
		klog.V(1).InfoS("error of PreFilter():", "ReasonK8sClient", ReasonK8sClient+err.Error())
		return result, nil
	}

	// PodListを取得
	podList, err := pl.clientset.CoreV1().Pods(rs.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.V(1).InfoS("error of PreFilter():", "ReasonK8sClient", ReasonK8sClient+err.Error())
		return result, nil
	}

	// NodeListを取得
	nodeList, err := pl.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.V(1).InfoS("error of PreFilter():", "ReasonK8sClient", ReasonK8sClient+err.Error())
		return result, nil
	}

	// schedulingSessionの更新
	if err := pl.updateSchedulingSession(ctx, rs, podList, nodeList); err != nil {
		klog.V(1).InfoS("error of PreFilter():", "ReasonSchedulingSession", ReasonSchedulingSession+err.Error())
		return result, nil
	}

	// 配置できるノードをリストする
	allocatableNodes, err := getAllocatableNodes(pl.schedulingSession[rs.Name], pod, nodeList)
	if err != nil {
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, ReasonShouldBeSpread)
	}

	klog.V(1).InfoS("list allocatable node", "allocatableNodes", allocatableNodes)

	nodenames := sets.NewString(allocatableNodes...)
	result.NodeNames = nodenames

	return result, nil
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

func (pl *PodSpread) updateSchedulingSession(ctx context.Context, rs *appsv1.ReplicaSet, podList *corev1.PodList, nodeList *corev1.NodeList) error {

	pl.mu.Lock()
	defer pl.mu.Unlock()

	// スケジュール対象Podが所属するReplicaSetのschedulingSessionがなければ初期化
	if _, ok := pl.schedulingSession[rs.Name]; !ok {
		totalReplicas := int(pointer.Int32Deref(rs.Spec.Replicas, 0))
		podspreadRate, ok := rs.Annotations[AnnotationPodSpreadRate]
		if !ok {
			return fmt.Errorf("ReplicaSet %s does not have annotation %s", rs.Name, AnnotationPodSpreadRate)
		}
		rate, err := strconv.ParseFloat(podspreadRate, 64)
		if err != nil {
			return fmt.Errorf("parse annotation %s got error: %w", AnnotationPodSpreadRate, err)
		}
		if !(0 <= rate && rate <= 1) {
			return fmt.Errorf("annotation %s is inavalid value", AnnotationPodSpreadRate)
		}
		redunduncy := int(float64(totalReplicas) * rate)

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

func getAllocatableNodes(ss *SchedulingSession, pod *corev1.Pod, nodeList *corev1.NodeList) ([]string, error) {

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

	var allocatableNodes []string

	// 十分に冗長配置されたので、どのノードにも配置できます
	if ss.TotalDeployed >= ss.Redunduncy {

		klog.V(1).InfoS("Reduduncy is enough")

		for _, node := range nodeList.Items {
			allocatableNodes = append(allocatableNodes, node.Name)
		}

		return allocatableNodes, nil
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
				allocatableNodes = append(allocatableNodes, node.Name)
			}
		}

		return allocatableNodes, nil
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
				allocatableNodes = append(allocatableNodes, node.Name)
			}
		}

		return allocatableNodes, nil
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
				allocatableNodes = append(allocatableNodes, node.Name)
			}
		}

		// klog.V(1).InfoS("decide denyNodes", "denyNodes", denyNodes)

		return allocatableNodes, nil
	default:
		return nil, fmt.Errorf("invalid SpreadMode :%v", ss.SpreadMode)
	}
}
