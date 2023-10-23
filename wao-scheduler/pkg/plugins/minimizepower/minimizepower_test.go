package minimizepower

import (
	"math"
	"reflect"
	"testing"

	framework "k8s.io/kubernetes/pkg/scheduler/framework"
)

func Test_PowerConsumptions2Scores(t *testing.T) {
	tests := []struct {
		name string
		arg  framework.NodeScoreList
		want framework.NodeScoreList
	}{
		{
			name: "1",
			arg: framework.NodeScoreList{
				{Name: "n0", Score: 0}, // lowest power increase, highest score
				{Name: "n1", Score: 50},
				{Name: "n2", Score: 100}, // worst power increase, lowest score
			},
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 50},
				{Name: "n2", Score: 0},
			},
		},
		{
			name: "2",
			arg: framework.NodeScoreList{
				{Name: "n0", Score: 20},
				{Name: "n1", Score: 30},
				{Name: "n2", Score: 40},
			},
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 50},
				{Name: "n2", Score: 0},
			},
		},
		{
			name: "3",
			arg: framework.NodeScoreList{
				{Name: "n0", Score: 2000},
				{Name: "n1", Score: 3000},
				{Name: "n2", Score: 4000},
			},
			want: framework.NodeScoreList{
				{Name: "n0", Score: 100},
				{Name: "n1", Score: 50},
				{Name: "n2", Score: 0},
			},
		},
		{
			name: "same",
			arg: framework.NodeScoreList{
				{Name: "n0", Score: 10},
				{Name: "n1", Score: 10},
				{Name: "n2", Score: 10},
			},
			want: framework.NodeScoreList{
				{Name: "n0", Score: 0},
				{Name: "n1", Score: 0},
				{Name: "n2", Score: 0},
			},
		},
		{
			name: "score_error",
			arg: framework.NodeScoreList{
				{Name: "n0", Score: math.MaxInt64},
				{Name: "n1", Score: math.MaxInt64},
				{Name: "n2", Score: math.MaxInt64},
			},
			want: framework.NodeScoreList{
				{Name: "n0", Score: 0},
				{Name: "n1", Score: 0},
				{Name: "n2", Score: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PowerConsumptions2Scores(tt.arg)
			if got := tt.arg; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PowerConsumptions2Scores() = %v, want %v", got, tt.want)
			} else {
				t.Logf("PowerConsumptions2Scores() = %v, want %v", got, tt.want)
			}
		})
	}
}
