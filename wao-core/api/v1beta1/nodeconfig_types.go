package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeConfigSpec defines the desired state of NodeConfig
type NodeConfigSpec struct {
	NodeName         string           `json:"nodeName"`
	MetricsCollector MetricsCollector `json:"metricsCollector"`
	Predictor        Predictor        `json:"predictor"`
}

type MetricsCollector struct {
	InletTemp EndpointTerm `json:"inletTemp"`
	DeltaP    EndpointTerm `json:"deltaP"`
}

type Predictor struct {
	// +optional
	PowerConsumption *EndpointTerm `json:"powerConsumption,omitempty"`
	// +optional
	PowerConsumptionEndpointProvider *EndpointTerm `json:"powerConsumptionEndpointProvider,omitempty"`
}

type EndpointTerm struct {
	// Type specifies the type of endpoint. This value means which client is used.
	Type string `json:"type"`
	// Endpoint specifies the endpoint URL. Behavior depends on the client specified by Type.
	Endpoint string `json:"endpoint"`
	// BasicAuthSecret specifies the name of the Secret in the same namespace used for basic auth. Some Types require this value.
	// +optional
	BasicAuthSecret *corev1.LocalObjectReference `json:"basicAuthSecret,omitempty"`
	// FetchInterval specifies the data retrieval interval. Some Types require this value, and behavior depends on the client.
	// +optional
	FetchInterval *metav1.Duration `json:"fetchInterval,omitempty"`
}

const (
	TypeFake                = "Fake"
	TypeRedfish             = "Redfish"
	TypeDPAPI               = "DifferentialPressureAPI"
	TypeV2InferenceProtocol = "V2InferenceProtocol"
)

// NodeConfigStatus defines the observed state of NodeConfig
type NodeConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NodeConfig is the Schema for the nodeconfigs API
type NodeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeConfigSpec   `json:"spec,omitempty"`
	Status NodeConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeConfigList contains a list of NodeConfig
type NodeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeConfig{}, &NodeConfigList{})
}
