/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/gardener/dependency-watchdog/internal/prober"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ProberMgr   prober.Manager
	ScaleGetter scale.ScalesGetter
	ProbeConfig *prober.Config
}

//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cluster, notFound, err := r.getCluster(ctx, req.Namespace, req.Name)

	if err != nil {
		logger.Error(err, "Unable to get the cluster resource, requeing for reconciliation", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	if notFound || cluster.DeletionTimestamp != nil {
		logger.V(4).Info("Cluster not found or marked for deletion, prober will be removed if present", "namespace", req.Namespace, "name", req.Name)
		r.deleteProber(req.Name)
		return ctrl.Result{}, nil
	}

	_, err = extensionscontroller.ShootFromCluster(cluster)
	if err != nil {
		logger.Error(err, "Error extracting shoot from cluster.", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	//TODO: update event part will come here.

	r.startProber(req.Name)

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) getCluster(ctx context.Context, namespace string, name string) (cluster *gardenerv1alpha1.Cluster, notFound bool, err error) {

	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cluster); err != nil {
		if errors.IsNotFound(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return cluster, false, nil
}

func (r *ClusterReconciler) deleteProber(key string) {
	_, ok := r.ProberMgr.GetProber(key)
	if ok {
		r.ProberMgr.Unregister(key)
	}
}

func (r *ClusterReconciler) startProber(key string) {
	_, ok := r.ProberMgr.GetProber(key)
	if !ok {
		deploymentScaler := prober.NewDeploymentScaler(key, r.ProbeConfig, r.Client, r.ScaleGetter)
		prober := prober.NewProber(key, r.ProbeConfig, r.Client, deploymentScaler)
		r.ProberMgr.Register(prober)
		prober.Run()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gardenerv1alpha1.Cluster{}).
		Complete(r)
}
