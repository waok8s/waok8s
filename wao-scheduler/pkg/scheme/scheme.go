package scheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeschedulerscheme "k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"

	"github.com/waok8s/wao-scheduler/pkg/plugins/minimizepower"
)

// https://github.com/kubernetes-sigs/scheduler-plugins/blob/v0.26.7/apis/config/scheme/scheme.go

var (
	Scheme = kubeschedulerscheme.Scheme
	Codecs = serializer.NewCodecFactory(Scheme, serializer.EnableStrict)
)

func init() {
	AddToScheme(Scheme)
}

func AddToScheme(scheme *runtime.Scheme) {
	utilruntime.Must(minimizepower.AddToScheme(scheme))
}
