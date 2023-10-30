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

	iet2 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.{{ add .IPv4.Octet3 10 }}.{{.IPv4.Octet4}}",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-worker-0"},
		FetchInterval:   &metav1.Duration{Duration: 10 * time.Second},
	}
	td2  = td0
	wet2 = EndpointTerm{
		Type:            "Redfish",
		Endpoint:        "https://10.0.10.1",
		BasicAuthSecret: &corev1.LocalObjectReference{Name: "redfish-basicauth-worker-0"},
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
		{"add", args{in: &iet2, data: td2}, &wet2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TemplateParseEndpointTerm(tt.args.in, tt.args.data); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TemplateParse() = %v, want %v", got, tt.want)
			}
		})
	}
}

var (
	labelHostname  = "kubernetes.io/hostname"
	testNode0Name  = "node-0"
	testNode1Name  = "node-1"
	testNode0Addr  = "10.0.0.100"
	testNode1Addr  = "10.0.0.101"
	testLabel      = "test-label"
	testLabelValue = "test-label-value"
	testNode0      = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNode0Name,
			Labels: map[string]string{labelHostname: testNode0Name, testLabel: testLabelValue}},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testNode0Addr}},
		},
	}
	testNode1 = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNode1Name,
			Labels: map[string]string{labelHostname: testNode1Name, testLabel: testLabelValue}},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testNode1Addr}},
		},
	}
	testTemplateData0 = TemplateData{
		Hostname: testNode0Name,
		IPv4:     TemplateDataIPv4{Address: testNode0Addr, Octet1: "10", Octet2: "0", Octet3: "0", Octet4: "100"},
	}
	testTemplateData1 = TemplateData{
		Hostname: testNode1Name,
		IPv4:     TemplateDataIPv4{Address: testNode1Addr, Octet1: "10", Octet2: "0", Octet3: "0", Octet4: "101"},
	}
)

func TestNewTemplateDataFromNode(t *testing.T) {
	type args struct {
		node corev1.Node
	}
	tests := []struct {
		name string
		args args
		want TemplateData
	}{
		{"ok0", args{node: testNode0}, testTemplateData0},
		{"ok1", args{node: testNode1}, testTemplateData1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewTemplateDataFromNode(tt.args.node); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewTemplateDataFromNode() = %v, want %v", got, tt.want)
			}
		})
	}
}
