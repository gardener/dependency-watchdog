package controllers

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/gardener/dependency-watchdog/internal/weeder"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EndpointReconciler reconciles an Endpoints object
type EndpointReconciler struct {
	client.Client
	WeederConfig            *wapi.Config
	WeederMgr               weeder.WeederManager
	MaxConcurrentReconciles int
}

// Reconcile listens to create/update events for `Endpoints` resources and manages weeder which shoot the dependent pods of the configured services, if necessary
func (r *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	//Get the endpoint object
	ep := v1.Endpoints{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &ep)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("Processing endpoint: ", "namespace", req.Namespace, "name", req.Name)

	//Check if the endpoint is ready, if not unregister existing weeder and return
	if !isReadyEndpointPresentInSubsets(ep.Subsets) {
		logger.Info("Endpoint %s does not have any endpoint subset. Skipping pod terminations.", ep.Name)
		r.WeederMgr.Unregister(ep.Name + req.Namespace)
		return ctrl.Result{}, nil
	}

	r.startWeeder(ctx, req.Name, req.Namespace, &ep)

	return ctrl.Result{}, nil
}

// startWeeder starts a new weeder for the endpoint
func (r *EndpointReconciler) startWeeder(ctx context.Context, name, namespace string, ep *v1.Endpoints) {
	uniqueName := name + "/" + namespace
	wLogger := log.FromContext(ctx).WithName(uniqueName).WithName("weeder")
	w := weeder.NewWeeder(ctx, namespace, r.WeederConfig, r.Client, ep, wLogger)
	//Register the weeder
	r.WeederMgr.Register(*w)
	go w.Run()
}

// isReadyEndpointPresentInSubsets checks if the endpoint resource have a subset of ready
// IP endpoints.
func isReadyEndpointPresentInSubsets(subsets []v1.EndpointSubset) bool {
	for _, subset := range subsets {
		if len(subset.Addresses) != 0 {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Endpoints{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
