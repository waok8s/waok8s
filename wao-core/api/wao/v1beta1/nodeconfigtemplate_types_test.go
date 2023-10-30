package v1beta1

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	iet0 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.100.{{.IPv4.Octet4}}",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-{{.Hostname}}"},
		FetchInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}
	td0 = TemplateData{
		Hostname: "worker-0",
		IPv4:     TemplateDataIPv4{Address: "10.0.0.1", Octet1: "10", Octet2: "0", Octet3: "0", Octet4: "1"},
	}
	wet0 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.100.1",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-worker-0"},
		FetchInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}

	iet1 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.100.{{.IPv4.Octet4}}",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-{{.Unknown}}"},
		FetchInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}
	td1  = td0
	wet1 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.100.1",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-{{.Unknown}}"},
		FetchInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}
)

func TestTemplateParseEndpointTerm(t *testing.T) {
	type args struct {
		in   *EndpointTerm
		data TemplateData
	}
	tests := []struct {
		name string
		args args
		want *EndpointTerm
	}{
		{"nil", args{in: nil, data: td0}, nil},
		{"ok", args{in: &iet0, data: td0}, &wet0},
		{"partially_fail", args{in: &iet1, data: td1}, &wet1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TemplateParseEndpointTerm(tt.args.in, tt.args.data); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TemplateParse() = %v, want %v", got, tt.want)
			}
		})
	}
}
