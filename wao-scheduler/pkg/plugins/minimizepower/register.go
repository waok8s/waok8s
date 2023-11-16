package minimizepower

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	schedschemev1 "k8s.io/kube-scheduler/config/v1"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
)

// https://github.com/kubernetes-sigs/scheduler-plugins/blob/v0.26.7/apis/config/v1/register.go

// register all kubescheduler.config.k8s.io APIs
var (
	SchemeGroupVersionV1Internal = schema.GroupVersion{Group: schedconfig.GroupName, Version: runtime.APIVersionInternal}
	SchemeGroupVersionV1B2       = schema.GroupVersion{Group: schedconfig.GroupName, Version: "v1beta2"}
	SchemeGroupVersionV1B3       = schema.GroupVersion{Group: schedconfig.GroupName, Version: "v1beta3"}
	SchemeGroupVersionV1         = schema.GroupVersion{Group: schedconfig.GroupName, Version: "v1"}
)

var (
	localSchemeBuilder = &schedschemev1.SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func newAddKnownTypes(gv schema.GroupVersion) func(*runtime.Scheme) error {
	return func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(gv,
			&MinimizePowerArgs{},
		)
		return nil
	}
}

func init() {
	localSchemeBuilder.Register(newAddKnownTypes(SchemeGroupVersionV1Internal))
	localSchemeBuilder.Register(newAddKnownTypes(SchemeGroupVersionV1B2))
	localSchemeBuilder.Register(newAddKnownTypes(SchemeGroupVersionV1B3))
	localSchemeBuilder.Register(newAddKnownTypes(SchemeGroupVersionV1))
}
