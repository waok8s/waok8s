package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeConfigTemplateSpec defines the desired state of NodeConfigTemplate
type NodeConfigTemplateSpec struct {
	NodeSelector     metav1.LabelSelector `json:"nodeSelector"`
	MetricsCollector MetricsCollector     `json:"metricsCollector"`
	Predictor        Predictor            `json:"predictor"`
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
