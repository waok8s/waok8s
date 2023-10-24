package wao

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NodeConfigTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	lg.Info("Reconcile")

	var nct waov1beta1.NodeConfigTemplate
	err := r.Get(ctx, req.NamespacedName, &nct)
	if errors.IsNotFound(err) {
		r.reconcileNodeConfigTemplateDeletion(ctx, req.NamespacedName)
		return ctrl.Result{}, nil
	}
	if err != nil {
		lg.Error(err, "unable to get NodeConfigTemplate")
		return ctrl.Result{}, err
	}
	if !nct.DeletionTimestamp.IsZero() {
		r.reconcileNodeConfigTemplateDeletion(ctx, req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if err := r.reconcileNodeConfigTemplate(ctx, req.NamespacedName, &nct); err != nil {
		lg.Error(err, "unable to reconcile NodeConfigTemplate", "obj", &nct)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeConfigTemplateReconciler) reconcileNodeConfigTemplateDeletion(ctx context.Context, name types.NamespacedName) error {
	lg := log.FromContext(ctx)
	lg.Info("reconcileNodeConfigTemplateDeletion")

	// TODO: implement

	return nil
}

func (r *NodeConfigTemplateReconciler) reconcileNodeConfigTemplate(ctx context.Context, name types.NamespacedName, nct *waov1beta1.NodeConfigTemplate) error {
	lg := log.FromContext(ctx)
	lg.Info("reconcileNodeConfigTemplate")

	// TODO: implement

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeConfigTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&waov1beta1.NodeConfigTemplate{}).
		Owns(&waov1beta1.NodeConfig{}).
		Complete(r)
}
