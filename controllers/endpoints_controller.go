package controllers

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/gardener/dependency-watchdog/internal/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

// EndpointReconciler reconciles an Endpoints object
type EndpointReconciler struct {
	client.Client
	SeedClient              kubernetes.Interface
	WeederConfig            *wapi.Config
	WeederMgr               weeder.Manager
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:resources=endpoints;events,verbs=create;get;update;patch;list;watch
// +kubebuilder:rbac:resources=pods,verbs=get;list;watch;delete

// Reconcile listens to create/update events for `Endpoints` resources and manages weeder which shoot the dependent pods of the configured services, if necessary
func (r *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	//Get the endpoint object
	var ep v1.Endpoints
	err := r.Client.Get(ctx, req.NamespacedName, &ep)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	log.Info("Starting a new weeder for endpoint, replacing old weeder, if any exists")
	r.startWeeder(ctx, log, req.Namespace, &ep)
	return ctrl.Result{}, nil
}

// startWeeder starts a new weeder for the endpoint
func (r *EndpointReconciler) startWeeder(ctx context.Context, logger logr.Logger, namespace string, ep *v1.Endpoints) {
	w := weeder.NewWeeder(ctx, namespace, r.WeederConfig, r.Client, r.SeedClient, ep, logger)
	// Register the weeder
	r.WeederMgr.Register(*w)
	go w.Run()
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Endpoints{}).
		WithEventFilter(predicate.And(
			predicate.ResourceVersionChangedPredicate{},
			MatchingEndpoints(r.WeederConfig.ServicesAndDependantSelectors), ReadyEndpoints(mgr.GetLogger()))).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
