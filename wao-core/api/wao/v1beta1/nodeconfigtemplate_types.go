package v1beta1

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TemplateData is a data structure for template rendering.
// This is not a part of CRD.
type TemplateData struct {
	// Hostname contains `kubernetes.io/hostname` label value.
	Hostname string
	// IPv4 contains address value of the first `InternalIP` in `status.addresses`.
	IPv4 TemplateDataIPv4
}

func NewTemplateDataFromNode(node corev1.Node) TemplateData {

	hostname := "undefined.example.com"
	if v, ok := node.Labels["kubernetes.io/hostname"]; ok {
		hostname = v
	}

	ipv4Address := "x.x.x.x"
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			ipv4Address = addr.Address
			break
		}
	}

	octets := []string{"x", "x", "x", "x"}
	if ss := strings.Split(ipv4Address, "."); len(ss) == 4 {
		octets = ss
	}

	return TemplateData{
		Hostname: hostname,
		IPv4: TemplateDataIPv4{
			Address: ipv4Address,
			Octet1:  octets[0],
			Octet2:  octets[1],
			Octet3:  octets[2],
			Octet4:  octets[3],
		},
	}
}

// TemplateDataIPv4 is a part of TemplateData.
type TemplateDataIPv4 struct {
	Address string
	Octet1  string
	Octet2  string
	Octet3  string
	Octet4  string
}

var tmpl *template.Template

func init() {
	tmpl = template.New("TemplateParseWithSprigFuncs").Funcs(sprig.FuncMap())
}

func TemplateParseString(s string, data TemplateData) (string, error) {
	t, err := tmpl.Parse(s)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func TemplateParseEndpointTerm(in *EndpointTerm, data TemplateData) *EndpointTerm {
	out := in.DeepCopy()

	if out == nil {
		return nil
	}

	// Type
	{
		v, err := TemplateParseString(in.Type, data)
		if err == nil {
			out.Type = v
		}
	}

	// Endpoint
	{
		v, err := TemplateParseString(in.Endpoint, data)
		if err == nil {
			out.Endpoint = v
		}
	}

	// BasicAuthSecret
	{
		if in.BasicAuthSecret != nil {
			v, err := TemplateParseString(in.BasicAuthSecret.Name, data)
			if err == nil {
				out.BasicAuthSecret.Name = v
			}
		}
	}

	// FetchInterval
	{
		// Templating is not supported as FetchInterval is a Duration type.
	}

	return out
}

func TemplateParseNodeConfig(nc *NodeConfig, data TemplateData) {
	nc.Spec.MetricsCollector.InletTemp = *TemplateParseEndpointTerm(&nc.Spec.MetricsCollector.InletTemp, data)
	nc.Spec.MetricsCollector.DeltaP = *TemplateParseEndpointTerm(&nc.Spec.MetricsCollector.DeltaP, data)

	nc.Spec.Predictor.PowerConsumption = TemplateParseEndpointTerm(nc.Spec.Predictor.PowerConsumption, data)
	nc.Spec.Predictor.PowerConsumptionEndpointProvider = TemplateParseEndpointTerm(nc.Spec.Predictor.PowerConsumptionEndpointProvider, data)
}

// NodeConfigTemplateSpec defines the desired state of NodeConfigTemplate
type NodeConfigTemplateSpec struct {
	// NodeSelector selects nodes to apply this template.
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`
	// Template is a template of NodeConfig.
	// You can use Go template syntax like `{{ .Hostname }}` `{{ .IPv4.Octet3 }}`
	// in string fields, see docs for more details.
	//
	// NOTE: template.nodeName is ignored.
	Template NodeConfigSpec `json:"template"`
}

// NodeConfigTemplateStatus defines the observed state of NodeConfigTemplate
type NodeConfigTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NodeConfigTemplate is the Schema for the nodeconfigtemplates API
type NodeConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeConfigTemplateSpec   `json:"spec,omitempty"`
	Status NodeConfigTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeConfigTemplateList contains a list of NodeConfigTemplate
type NodeConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeConfigTemplate{}, &NodeConfigTemplateList{})
}
