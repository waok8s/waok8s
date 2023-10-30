package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	"github.com/waok8s/wao-core/pkg/metrics"
	"github.com/waok8s/wao-core/pkg/metrics/dpapi"
	"github.com/waok8s/wao-core/pkg/metrics/fake"
	"github.com/waok8s/wao-core/pkg/metrics/redfish"
	"github.com/waok8s/wao-core/pkg/util"
)

var (
	Scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))
	utilruntime.Must(waov1beta1.AddToScheme(Scheme))
}

// NodeConfigReconciler reconciles a NodeConfig object.
//
// NOTE: This reconciler is used in wao-metrics-adaptor instead of the controller. So RBAC rules below should be applied to wao-metrics-adaptor.
// kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs,verbs=get;list;watch;create;update;patch;delete
// kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs/status,verbs=get;update;patch
// kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs/finalizers,verbs=update
// kubebuilder:rbac:groups=core,namespace=wao-system,resources=secrets,verbs=get
type NodeConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// SecretClient is used to get namespace scoped Secrets.
	// See: NodeConfigReconciler.getBasicAuthFromSecret()
	SecretClient kubernetes.Interface

	MetricsCollector *metrics.Collector
	MetricsStore     *metrics.Store
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NodeConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithValues("func", "Reconcile")
	lg.Info("called")

	var nc waov1beta1.NodeConfig
	err := r.Get(ctx, req.NamespacedName, &nc)
	if errors.IsNotFound(err) {
		r.reconcileNodeConfigDeletion(ctx, req.NamespacedName)
		return ctrl.Result{}, nil
	}
	if err != nil {
		lg.Error(err, "unable to get NodeConfig")
		return ctrl.Result{}, err
	}
	if !nc.DeletionTimestamp.IsZero() {
		r.reconcileNodeConfigDeletion(ctx, req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if err := r.reconcileNodeConfig(ctx, req.NamespacedName, &nc); err != nil {
		lg.Error(err, "unable to reconcile NodeConfig", "obj", &nc)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeConfigReconciler) reconcileNodeConfigDeletion(ctx context.Context, objKey types.NamespacedName) {
	lg := log.FromContext(ctx).WithValues("func", "reconcileNodeConfigDeletion")
	lg.Info("called")

	for _, vt := range metrics.ValueTypes {
		r.MetricsCollector.Unregister(metrics.CollectorKey(objKey, vt))
	}
}

func (r *NodeConfigReconciler) getBasicAuthFromSecret(ctx context.Context, namespace string, ref *corev1.LocalObjectReference) (username, password string) {
	lg := log.FromContext(ctx).WithValues("func", "getBasicAuthFromSecret")
	lg.Info("called")

	if ref == nil || ref.Name == "" {
		return
	}

	// NOTE: This is a workaround. How to use controller-runtime client to get namespace scoped Secrets?
	secret, err := r.SecretClient.CoreV1().Secrets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		lg.Error(err, "unable to get Secret so skip basic auth", "namespace", namespace, "name", ref.Name)
		return "", ""
	}

	// NOTE: RBAC error and crash. Why?
	// secret := &corev1.Secret{}
	// if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
	// 	lg.Error(err, "unable to get Secret so skip basic auth", "namespace", namespace, "name", ref.Name)
	// 	return "", ""
	// }

	username = string(secret.Data["username"])
	password = string(secret.Data["password"])

	return
}

const (
	DefaultFetchInterval = 15 * time.Second
)

func defaultEndpointTerm(et *waov1beta1.EndpointTerm) {
	if et.FetchInterval == nil {
		et.FetchInterval = &metav1.Duration{Duration: DefaultFetchInterval}
	}
}

func (r *NodeConfigReconciler) reconcileNodeConfig(ctx context.Context, objKey types.NamespacedName, nc *waov1beta1.NodeConfig) error {
	lg := log.FromContext(ctx).WithValues("func", "reconcileNodeConfig")
	lg.Info("called")

	// setup inlet temp agent
	{
		conf := nc.Spec.MetricsCollector.InletTemp
		defaultEndpointTerm(&conf)
		var agent metrics.Agent
		username, password := r.getBasicAuthFromSecret(ctx, objKey.Namespace, conf.BasicAuthSecret)
		fetchTimeout := 3 * time.Second
		switch conf.Type {
		case waov1beta1.TypeFake:
			agent = fake.NewInletTempAgent(15.5, nil, 100*time.Millisecond) // fake agent always returns this value
		case waov1beta1.TypeRedfish:
			insecureSkipVerify := true
			requestTimeout := fetchTimeout - 1*time.Second
			requestEditorFns := []util.RequestEditorFn{
				util.WithBasicAuth(username, password),
				util.WithCurlLogger(slog.With("func", "WithCurlLogger(RedfishClient.Fetch)", "node", nc.Spec.NodeName)),
			}
			agent = redfish.NewInletTempAgent(conf.Endpoint, redfish.TypeAutoDetect, insecureSkipVerify, requestTimeout, requestEditorFns...)
		default:
			return fmt.Errorf("unsupported metricsCollector.inletTemp.type: %s", conf.Type)
		}
		r.MetricsCollector.Register(metrics.CollectorKey(objKey, metrics.ValueInletTemperature), agent, r.MetricsStore, nc.Spec.NodeName, conf.FetchInterval.Duration, fetchTimeout)
	}

	// setup delta pressure agent
	{
		conf := nc.Spec.MetricsCollector.DeltaP
		defaultEndpointTerm(&conf)
		var agent metrics.Agent
		username, password := r.getBasicAuthFromSecret(ctx, objKey.Namespace, conf.BasicAuthSecret)
		fetchTimeout := 3 * time.Second
		switch conf.Type {
		case waov1beta1.TypeFake:
			agent = fake.NewDeltaPAgent(7.5, nil, 100*time.Millisecond) // fake agent always returns this value
		case waov1beta1.TypeDPAPI:
			insecureSkipVerify := true
			requestTimeout := fetchTimeout - 1*time.Second
			requestEditorFns := []util.RequestEditorFn{
				util.WithBasicAuth(username, password),
				util.WithCurlLogger(slog.With("func", "WithCurlLogger(DifferentialPressureAPIClient.Fetch)", "node", nc.Spec.NodeName)),
			}
			agent = dpapi.NewDeltaPAgent(conf.Endpoint, "", nc.Spec.NodeName, "", insecureSkipVerify, requestTimeout, requestEditorFns...)
		default:
			return fmt.Errorf("unsupported metricsCollector.deltaP.type: %s", conf.Type)
		}
		r.MetricsCollector.Register(metrics.CollectorKey(objKey, metrics.ValueDeltaPressure), agent, r.MetricsStore, nc.Spec.NodeName, conf.FetchInterval.Duration, fetchTimeout)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&waov1beta1.NodeConfig{}).
		Complete(r)
}
