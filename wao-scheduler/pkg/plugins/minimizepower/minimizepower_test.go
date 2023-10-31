package minimizepower

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

func Test_PowerConsumptions2Scores(t *testing.T) {
	tests := []struct {
		name       string
		input      framework.NodeScoreList
		baseScore  int64
		replaceMap map[int64]int64
		want       framework.NodeScoreList
	}{
		{
			name: "1",
			input: framework.NodeScoreList{
				{Name: "n0", Score: 0}, // lowest power increase, highest score
				{Name: "n1", Score: 50},
				{Name: "n2", Score: 100}, // worst power increase, lowest score
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 60},
				{Name: "n2", Score: 20},
			},
		},
		{
			name: "2",
			input: framework.NodeScoreList{
				{Name: "n0", Score: 20},
				{Name: "n1", Score: 30},
				{Name: "n2", Score: 40},
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 60},
				{Name: "n2", Score: 20},
			},
		},
		{
			name: "3",
			input: framework.NodeScoreList{
				{Name: "n0", Score: 2000},
				{Name: "n1", Score: 3000},
				{Name: "n2", Score: 4000},
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 60},
				{Name: "n2", Score: 20},
			},
		},
		{
			name: "same",
			input: framework.NodeScoreList{
				{Name: "n0", Score: 33},
				{Name: "n1", Score: 33},
				{Name: "n2", Score: 33},
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 20},
				{Name: "n1", Score: 20},
				{Name: "n2", Score: 20},
			},
		},
		{
			name: "score_error",
			input: framework.NodeScoreList{
				{Name: "n0", Score: ScoreError},
				{Name: "n1", Score: ScoreError},
				{Name: "n2", Score: ScoreMax},
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 0},
				{Name: "n1", Score: 0},
				{Name: "n2", Score: 1},
			},
		},
		{
			name: "with_special_scores",
			input: framework.NodeScoreList{
				{Name: "n0", Score: ScoreError},
				{Name: "n1", Score: ScoreMax},
				{Name: "n2", Score: 10},
				{Name: "n3", Score: 20},
				{Name: "n4", Score: 30},
			},
			baseScore:  ScoreBase,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 0},
				{Name: "n1", Score: 1},
				{Name: "n2", Score: 100},
				{Name: "n3", Score: 60},
				{Name: "n4", Score: 20},
			},
		},
		{
			name: "score_base_50",
			input: framework.NodeScoreList{
				{Name: "n0", Score: ScoreError},
				{Name: "n1", Score: ScoreMax},
				{Name: "n2", Score: 10},
				{Name: "n3", Score: 20},
				{Name: "n4", Score: 30},
			},
			baseScore:  50,
			replaceMap: ScoreReplaceMap,
			want: framework.NodeScoreList{
				{Name: "n0", Score: 0},
				{Name: "n1", Score: 1},
				{Name: "n2", Score: 100},
				{Name: "n3", Score: 75},
				{Name: "n4", Score: 50},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PowerConsumptions2Scores(tt.input, tt.baseScore, tt.replaceMap)
			if got := tt.input; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PowerConsumptions2Scores() = %v, want %v", got, tt.want)
			} else {
				t.Logf("PowerConsumptions2Scores() = %v, want %v", got, tt.want)
			}
		})
	}
}

func podWithResourceCPU(requests []string, limits []string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			InitContainers:      []corev1.Container{},
			Containers:          []corev1.Container{},
			EphemeralContainers: []corev1.EphemeralContainer{},
		},
	}

	if len(requests) != len(limits) {
		panic("len(reqs) != len(limits)")
	}

	for i := range requests {
		container := corev1.Container{
			Name:      fmt.Sprintf("%s-%d", pod.Name, i),
			Resources: corev1.ResourceRequirements{},
		}
		req := requests[i]
		if req != "" {
			container.Resources.Requests = corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse(req),
			}
		}
		lim := limits[i]
		if lim != "" {
			container.Resources.Limits = corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse(lim),
			}
		}
		pod.Spec.Containers = append(pod.Spec.Containers, container)
	}

	return pod
}

var Epsilon float64 = 0.00000001

func floatEquals(a, b float64) bool { return math.Abs(a-b) < Epsilon }

func TestPodCPURequestOrLimit(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name  string
		args  args
		wantV float64
	}{
		{name: "1container", args: args{pod: podWithResourceCPU([]string{"100m"}, []string{"200m"})}, wantV: 0.1},
		{name: "2containers", args: args{pod: podWithResourceCPU([]string{"100m", "200m"}, []string{"200m", "400m"})}, wantV: 0.3},
		{name: "requests_only", args: args{pod: podWithResourceCPU([]string{"100m", "200m"}, []string{"", ""})}, wantV: 0.3},
		{name: "limits_only", args: args{pod: podWithResourceCPU([]string{"", ""}, []string{"200m", "400m"})}, wantV: 0.6},
		{name: "mixed", args: args{pod: podWithResourceCPU([]string{"100m", ""}, []string{"", "400m"})}, wantV: 0.5},
		{name: "empty", args: args{pod: podWithResourceCPU([]string{"", ""}, []string{"", ""})}, wantV: 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotV := PodCPURequestOrLimit(tt.args.pod); !floatEquals(gotV, tt.wantV) {
				t.Errorf("PodCPURequestOrLimit() = %v, want %v", gotV, tt.wantV)
			}
		})
	}
}
