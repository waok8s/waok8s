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
	"k8s.io/utils/ptr"
)

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

type SchedulingSession struct {
	TotalReplicas int
	Redunduncy    int
	DeployedNodes map[string]int
	TotalDeployed int
	SpreadMode    SpreadMode
	SpreadInfo    SpreadInfo
}

type PodSpread struct {
	mu                sync.Mutex
	schedulingSession map[string]*SchedulingSession
	clientset         kubernetes.Interface
}

var _ framework.PreFilterPlugin = (*PodSpread)(nil)

var (
	Name = "PodSpread"

	ReasonInvalidAnnotation         = "invalid annotation"
	ReasonEmptyNodeName             = "node not found"
	ReasonNotControlledByReplicaSet = "the pod is not controlled by any ReplicaSet"
	ReasonK8sClient                 = "skip this plugin as k8s client got error"
	ReasonSchedulingSession         = "skip this plugin as SchedulingSession update failed"
)

// Name returns name of the plugin. It is used in logs, etc.
func (*PodSpread) Name() string { return Name }

const (
	AnnotationPodSpreadRate = "waok8s.github.io/podspread-rate"
)

// New initializes a new plugin and returns it.
func New(_ context.Context, _ runtime.Object, fh framework.Handle) (framework.Plugin, error) {
	return &PodSpread{
		schedulingSession: map[string]*SchedulingSession{},
		clientset:         fh.ClientSet(),
	}, nil
}

// PreFilterExtensions do not exist for this plugin.
func (pl *PodSpread) PreFilterExtensions() framework.PreFilterExtensions { return nil }

// PreFilter invoked at the prefilter extension point.
func (pl *PodSpread) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	// pod will be rejected if error is returned, so skip this plugin for some cases
	result := &framework.PreFilterResult{NodeNames: nil}

	_, err := parseAnnotationPodSpreadRate(pod.Annotations)
	if err != nil {
		klog.InfoS("PodSpread.PreFilter skipped", "reason", ReasonInvalidAnnotation)
		return result, nil
	}

	// get ReplicaSet that controls this Pod
	replicaSetName := ""
	for _, ref := range pod.OwnerReferences {
		if ptr.Deref[bool](ref.Controller, false) && ref.Kind == "ReplicaSet" {
			replicaSetName = ref.Name
		}
	}
	if replicaSetName == "" {
		klog.InfoS("PodSpread.PreFilter skipped", "reason", ReasonNotControlledByReplicaSet)
		return result, nil
	}
	rs, err := pl.clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, replicaSetName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "PodSpread.PreFilter skipped", "reason", ReasonK8sClient)
		return result, nil
	}

	// get podList and nodeList, and then do updateSchedulingSession
	podList, err := pl.clientset.CoreV1().Pods(rs.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "PodSpread.PreFilter skipped", "reason", ReasonK8sClient)
		return result, nil
	}
	nodeList, err := pl.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "PodSpread.PreFilter skipped", "reason", ReasonK8sClient)
		return result, nil
	}
	if err := pl.updateSchedulingSession(ctx, rs, podList, nodeList); err != nil {
		klog.ErrorS(err, "PodSpread.PreFilter skipped", "reason", ReasonSchedulingSession)
		return result, nil
	}

	// list allocatable nodes
	allocatableNodes := getAllocatableNodes(pl.schedulingSession[rs.Name], pod, nodeList)
	klog.InfoS("list allocatable node", "allocatableNodes", allocatableNodes)

	nodenames := sets.New[string](allocatableNodes...)
	result.NodeNames = nodenames

	return result, nil
}

const (
	// https://kubernetes.io/docs/reference/labels-annotations-taints/

	labelZone         = "topology.kubernetes.io/zone"
	labelRegion       = "topology.kubernetes.io/region"
	labelControlPlane = "node-role.kubernetes.io/control-plane"
)

// getSpreadMode returns SpreadMode and SpreadInfo.
//
// NOTE: Control plane only areas should be excluded from scheduling, but not implemented yet.
// Then the signature will be like: getSpreadMode(nodeList *corev1.NodeList, isControlPlaneSchedulable bool) (mode SpreadMode, regions, zones SpreadInfo)
func getSpreadMode(nodeList *corev1.NodeList) (mode SpreadMode, regions, zones SpreadInfo) {
	zones = SpreadInfo{} // key: [zone][node], value: is master node?
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

	regions = SpreadInfo{} // key: [region][node], value: is master node?
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

func parseAnnotationPodSpreadRate(annotations map[string]string) (float64, error) {
	podspreadRate, ok := annotations[AnnotationPodSpreadRate]
	if !ok {
		return 0, fmt.Errorf("annotation %s not found", AnnotationPodSpreadRate)
	}
	v, err := strconv.ParseFloat(podspreadRate, 64)
	if err != nil {
		return 0, fmt.Errorf("parse annotation %s got error: %w", AnnotationPodSpreadRate, err)
	}
	if v < 0 || v > 1 {
		return 0, fmt.Errorf("annotation %s should be 0 <= v <= 1 but got %v", AnnotationPodSpreadRate, v)
	}
	return v, nil
}

func (pl *PodSpread) updateSchedulingSession(ctx context.Context, rs *appsv1.ReplicaSet, podList *corev1.PodList, nodeList *corev1.NodeList) error {

	pl.mu.Lock()
	defer pl.mu.Unlock()

	// init schedulingSession for this ReplicaSet if not exists
	if _, ok := pl.schedulingSession[rs.Name]; !ok {
		totalReplicas := int(ptr.Deref[int32](rs.Spec.Replicas, 0))

		// TODO: check Pod annotation first?
		rate, err := parseAnnotationPodSpreadRate(rs.Annotations)
		if err != nil {
			return err
		}
		redunduncy := int(float64(totalReplicas) * rate)

		pl.schedulingSession[rs.Name] = &SchedulingSession{
			TotalReplicas: totalReplicas,
			Redunduncy:    redunduncy,
		}
	}

	// get schedulingSession
	ss := pl.schedulingSession[rs.Name]
	klog.InfoS("get scheduling session", "ss", ss)

	// update DeployedNodes
	ss.DeployedNodes = map[string]int{}
	for _, n := range nodeList.Items {
		if _, ok := ss.DeployedNodes[n.Name]; !ok {
			ss.DeployedNodes[n.Name] = 0
		}
	}
	ss.setDeployNodes(rs, podList)

	// update SpreadMode and SpreadInfo
	mode, regions, zones := getSpreadMode(nodeList)
	ss.SpreadMode = mode
	switch mode {
	case SpreadModeRegion:
		ss.SpreadInfo = regions
	case SpreadModeZone:
		ss.SpreadInfo = zones
	default:
		ss.SpreadInfo = SpreadInfo{}
	}

	return nil
}

func (ss *SchedulingSession) setDeployNodes(rs *appsv1.ReplicaSet, podList *corev1.PodList) {
	ss.TotalDeployed = 0

	// If there is Type:PodScheduled,Status:True in Pod Conditions, the node where the Pod is placed is determined.
	// Why PodScheduled? https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
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

// getAllocatableNodes returns a list of nodes that can be scheduled.
//
// NOTE: Control plane only areas should be excluded from scheduling, but not tested yet.
func getAllocatableNodes(ss *SchedulingSession, pod *corev1.Pod, nodeList *corev1.NodeList) []string {

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

	// pod can be deployed to any node as it is sufficiently redundant
	if ss.TotalDeployed >= ss.Redunduncy {
		klog.InfoS("sufficiently redundant")
		for _, node := range nodeList.Items {
			allocatableNodes = append(allocatableNodes, node.Name)
		}
		return allocatableNodes
	}

	// Check if the pod is a control plane pod.
	// https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	//
	// Only control plane nodes:
	// ```
	// nodeSelector:
	//   node-role.kubernetes.io/control-plane: ""
	// ```
	//
	// Control plane nodes are also OK:
	// ```
	// tolerations:
	//   - key: "node-role.kubernetes.io/control-plane"
	// 	   operator: "Exists"
	// 	   effect: "NoSchedule"
	// ```
	isControlPlaneSchedulable := false
	for _, t := range pod.Spec.Tolerations {
		if t.Key == labelControlPlane && t.Operator == corev1.TolerationOpExists && t.Effect == corev1.TaintEffectNoSchedule {
			isControlPlaneSchedulable = true
		}
	}

	switch ss.SpreadMode {
	case SpreadModeRegion, SpreadModeZone:
		areaDeployed := map[string]int{}
		var minAreas []string
		cpOnlyAreas := map[string]struct{}{}

		// Get the total number of pods controlled by the ReplicaSet in each area.
		// Areas with only control plane nodes are excluded from scheduling.
		for area, areaSpreadInfo := range ss.SpreadInfo {
			deployed := 0
			hasNonCP := false
			for node, cp := range areaSpreadInfo {
				if !cp {
					hasNonCP = true
				}
				deployed += ss.DeployedNodes[node]
			}
			if !hasNonCP {
				cpOnlyAreas[area] = struct{}{}
			}
			areaDeployed[area] = deployed
		}

		// Get the minimum number of pods controlled by the ReplicaSet in all areas.
		minNum := math.MaxInt
		for _, num := range areaDeployed {
			if num < minNum {
				minNum = num
			}
		}

		// Get the list of areas with the minimum number of pods controlled by the ReplicaSet deployed.
		for area, deployed := range areaDeployed {
			if !isControlPlaneSchedulable {
				if _, ok := cpOnlyAreas[area]; ok {
					continue
				}
			}
			if deployed == minNum {
				minAreas = append(minAreas, area)
			}
		}

		allowedNodes := map[string]struct{}{}

		for _, area := range minAreas {
			for node, cp := range ss.SpreadInfo[area] {
				if cp && !isControlPlaneSchedulable {
					continue
				}
				allowedNodes[node] = struct{}{}
			}
		}

		for nodeName := range allowedNodes {
			allocatableNodes = append(allocatableNodes, nodeName)
		}
		return allocatableNodes

	default: // SpreadModeNode
		allowedNodes := map[string]struct{}{}
		minDeployed := math.MaxInt

		for node, deployed := range ss.DeployedNodes {
			if isControlPlane(node) && !isControlPlaneSchedulable {
				continue
			}
			klog.InfoS("make node map", "node", node, "deployed", deployed)
			if deployed < minDeployed {
				minDeployed = deployed
			}
		}

		for node, deployed := range ss.DeployedNodes {
			if isControlPlane(node) && !isControlPlaneSchedulable {
				continue
			}
			if deployed == minDeployed {
				allowedNodes[node] = struct{}{}
			}
		}

		for nodeName := range allowedNodes {
			allocatableNodes = append(allocatableNodes, nodeName)
		}
		return allocatableNodes
	}
}
