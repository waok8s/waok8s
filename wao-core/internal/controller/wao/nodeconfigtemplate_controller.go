package wao

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
)

// NodeConfigTemplateReconciler reconciles a NodeConfigTemplate object
type NodeConfigTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigtemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigtemplates/finalizers,verbs=update
//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=wao.bitmedia.co.jp,resources=nodeconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NodeConfigTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithValues("func", "Reconcile")
	lg.Info("called")

	var nct waov1beta1.NodeConfigTemplate
	err := r.Get(ctx, req.NamespacedName, &nct)
	if errors.IsNotFound(err) {
		// GC will delete NodeConfigs created by this NodeConfigTemplate
		return ctrl.Result{}, nil
	}
	if err != nil {
		lg.Error(err, "unable to get NodeConfigTemplate")
		return ctrl.Result{}, err
	}
	if !nct.DeletionTimestamp.IsZero() {
		// GC will delete NodeConfigs created by this NodeConfigTemplate
		return ctrl.Result{}, nil
	}

	if err := r.reconcileNodeConfigTemplate(ctx, req.NamespacedName, &nct); err != nil {
		lg.Error(err, "unable to reconcile NodeConfigTemplate", "obj", &nct)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeConfigTemplateReconciler) reconcileNodeConfigTemplate(ctx context.Context, name types.NamespacedName, nct *waov1beta1.NodeConfigTemplate) error {
	lg := log.FromContext(ctx).WithValues("func", "reconcileNodeConfigTemplate")
	lg.Info("called")

	s, err := metav1.LabelSelectorAsSelector(&nct.Spec.NodeSelector)
	if err != nil {
		lg.Error(err, "unable to convert NodeSelector to Selector", "obj", nct)
		return err
	}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, client.MatchingLabelsSelector{Selector: s}); err != nil {
		lg.Error(err, "unable to list Nodes", "obj", nct)
		return err
	}

	for _, node := range nodes.Items {
		if err := r.reconcileNodeConfig(ctx, nct, node); err != nil {
			lg.Error(err, "unable to reconcile NodeConfig", "obj", nct, "node", node.Name)
			continue
		}
	}

	return nil
}

func (r *NodeConfigTemplateReconciler) reconcileNodeConfig(ctx context.Context, nct *waov1beta1.NodeConfigTemplate, node corev1.Node) error {
	lg := log.FromContext(ctx).WithValues("func", "reconcileNodeConfig")
	lg.Info("called")

	ncName := fmt.Sprintf("%s-%s", nct.Name, node.Name)
	ncNamespace := nct.Namespace
	ncObj := types.NamespacedName{Namespace: ncNamespace, Name: ncName}

	nc := &waov1beta1.NodeConfig{}
	nc.SetName(ncName)
	nc.SetNamespace(ncNamespace)

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, nc, func() error {
		nc.Spec.NodeName = node.Name
		nc.Spec.MetricsCollector = *nct.Spec.MetricsCollector.DeepCopy()
		nc.Spec.Predictor = *nct.Spec.Predictor.DeepCopy()
		waov1beta1.TemplateParseNodeConfig(nc, waov1beta1.NewTemplateDataFromNode(node))
		return ctrl.SetControllerReference(nct, nc, r.Scheme)
	})
	if err != nil {
		lg.Error(err, "unable to create or update NodeConfig", "obj", ncObj)
		return err
	}
	lg.Info("NodeConfig reconciled", "obj", ncObj, "op", op)
	return nil
}

func (r *NodeConfigTemplateReconciler) mapFuncNodeToNodeConfigTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	lg := log.FromContext(ctx).WithValues("func", "mapFuncNodeToNodeConfigTemplate")
	lg.Info("called")

	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), &node); err != nil {
		lg.Error(err, "unable to get Node", "obj", obj)
		return nil
	}

	var ncts waov1beta1.NodeConfigTemplateList
	if err := r.List(ctx, &ncts); err != nil {
		lg.Error(err, "unable to list NodeConfigTemplates")
		return nil
	}

	for _, nct := range ncts.Items {
		s, err := metav1.LabelSelectorAsSelector(&nct.Spec.NodeSelector)
		if err != nil {
			lg.Error(err, "unable to convert NodeSelector to Selector", "obj", &nct)
			continue
		}
		if s.Matches(labels.Set(node.Labels)) {
			nctObj := types.NamespacedName{Namespace: nct.Namespace, Name: nct.Name}
			lg.Info("NodeConfigTemplate matched", "node", node.Name, "obj", nctObj)
			return []reconcile.Request{{NamespacedName: nctObj}}
		}
	}

	lg.Info("NodeConfigTemplate not matched", "node", node.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeConfigTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&waov1beta1.NodeConfigTemplate{}).
		Owns(&waov1beta1.NodeConfig{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapFuncNodeToNodeConfigTemplate)).
		Complete(r)
}
