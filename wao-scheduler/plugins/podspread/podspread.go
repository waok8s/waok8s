package podspread

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type PodSpread struct{}

var _ framework.FilterPlugin = &PodSpread{}

const (
	Name = "PodSpread"
)

func (*PodSpread) Name() string { return Name }

func (pl *PodSpread) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	panic("TODO: not yet implemented")
}

func New(plArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	panic("TODO: not yet implemented")
}
