package nftables

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_decodeSvcPortNameString(t *testing.T) {
	type args struct {
		svcPortNameString string
	}
	tests := []struct {
		name          string
		args          args
		wantNamespace string
		wantSvcName   string
		wantPortName  string
	}{
		{"default/nginx", args{"default/nginx"}, "default", "nginx", ""},
		{"default/nginx:http", args{"default/nginx:http"}, "default", "nginx", "http"},
		{"default/nginx:https", args{"default/nginx:https"}, "default", "nginx", "https"},
		{"error1", args{"nginx"}, "", "", ""},
		{"error2", args{"nginx:http"}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotSvcName, gotPortName := decodeSvcPortNameString(tt.args.svcPortNameString)
			if gotNamespace != tt.wantNamespace {
				t.Errorf("decodeSvcPortNameString() gotNamespace = %v, want %v", gotNamespace, tt.wantNamespace)
			}
			if gotSvcName != tt.wantSvcName {
				t.Errorf("decodeSvcPortNameString() gotSvcName = %v, want %v", gotSvcName, tt.wantSvcName)
			}
			if gotPortName != tt.wantPortName {
				t.Errorf("decodeSvcPortNameString() gotPortName = %v, want %v", gotPortName, tt.wantPortName)
			}
		})
	}
}

func Test_normalizeScores(t *testing.T) {
	type args struct {
		watts map[string]int
	}
	tests := []struct {
		name string
		args args
		want map[string]int
	}{
		{"empty", args{map[string]int{}}, map[string]int{}},
		{"-1", args{map[string]int{"10.0.0.1": -1}}, map[string]int{}},
		{"0", args{map[string]int{"10.0.0.1": 0}}, map[string]int{"10.0.0.1": 100}},
		{"1", args{map[string]int{"10.0.0.1": 1}}, map[string]int{"10.0.0.1": 100}},
		{"normal", args{map[string]int{"10.0.0.1": 200, "10.0.0.2": 250, "10.0.0.3": 300}},
			map[string]int{"10.0.0.1": 100, "10.0.0.2": 80, "10.0.0.3": 67}},
		{"big", args{map[string]int{"10.0.0.1": 200, "10.0.0.2": 200_000_000}},
			map[string]int{"10.0.0.1": 100, "10.0.0.2": 0}},
		{"normal2", args{map[string]int{"10.0.0.1": 5, "10.0.0.2": 2, "10.0.0.3": -123, "10.0.0.4": 0, "10.0.0.5": 5, "10.0.0.6": 0}},
			map[string]int{"10.0.0.1": 17, "10.0.0.2": 33, "10.0.0.4": 100, "10.0.0.5": 17, "10.0.0.6": 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeScores(tt.args.watts)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("normalizeScores() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
