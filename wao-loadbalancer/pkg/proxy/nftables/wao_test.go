package nftables

import "testing"

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
