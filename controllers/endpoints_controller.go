package controllers

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/gardener/dependency-watchdog/internal/weeder"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// EndpointReconciler reconciles an Endpoints object
type EndpointReconciler struct {
	client.Client
	WeederConfig            *wapi.Config
	WeederMgr               weeder.Manager
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups,resources=endpoints;events,verbs=create;get;update;patch;list;watch
// +kubebuilder:rbac:groups,resources=pods,verbs=get;list;watch;delete

// Reconcile listens to create/update events for `Endpoints` resources and manages weeder which shoot the dependent pods of the configured services, if necessary
func (r *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	//Get the endpoint object
	var ep v1.Endpoints
	err := r.Client.Get(ctx, req.NamespacedName, &ep)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10}, err
	}
	logger.Info("Processing endpoint: ", "namespace", req.Namespace, "name", req.Name)
	r.startWeeder(ctx, req.Name, req.Namespace, &ep)
	return ctrl.Result{}, nil
}

// startWeeder starts a new weeder for the endpoint
func (r *EndpointReconciler) startWeeder(ctx context.Context, name, namespace string, ep *v1.Endpoints) {
	uniqueName := name + "/" + namespace
	wLogger := log.FromContext(ctx).WithName(uniqueName).WithName("weeder")
	w := weeder.NewWeeder(ctx, namespace, r.WeederConfig, r.Client, ep, wLogger)
	// Register the weeder
	r.WeederMgr.Register(*w)
	go w.Run()
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Endpoints{}).
		WithEventFilter(predicate.And(predicate.ResourceVersionChangedPredicate{}, ReadyEndpoints(), MatchingEndpoints(r.WeederConfig.ServicesAndDependantSelectors))).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
