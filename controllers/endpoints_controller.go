package controllers

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointReconciler struct {
	client.Client
	WeederConfig *wapi.Config
}

func (r *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//Get the endpoint object
	//Check if the endpoint is ready, if not unregister existing weeder and return
	//Register the weeder
	//Call weeder.Run
}
