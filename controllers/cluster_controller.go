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

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"

	apiprober "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/prober"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
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
	ProbeConfig *apiprober.Config
}

//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters/status,verbs=get

// Reconcile listens to create/update/delete events for `Cluster` resources and
// manages probes for the shoot control namespace for these clusters by looking at the cluster state.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cluster, notFound, err := r.getCluster(ctx, req.Namespace, req.Name)
	if err != nil {
		logger.Error(err, "Unable to get the cluster resource, requeing for reconciliation", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}
	// If the cluster is not found then any existing probes if present will be unregistered
	if notFound {
		logger.V(4).Info("Cluster not found, any existing probes will be removed if present", "namespace", req.Namespace, "name", req.Name)
		r.ProberMgr.Unregister(req.Name)
		return ctrl.Result{}, nil
	}

	shoot, err := extensionscontroller.ShootFromCluster(cluster)
	if err != nil {
		logger.Error(err, "Error extracting shoot from cluster.", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	// If shoot is marked for deletion then any existing probes will be unregistered
	if shoot.DeletionTimestamp != nil {
		logger.V(4).Info("Cluster has been marked for deletion, any existing probes will be removed if present", "namespace", req.Namespace, "name", req.Name)
		r.ProberMgr.Unregister(req.Name)
		return ctrl.Result{}, nil
	}

	// if hibernation is enabled then we will remove any existing prober. Any resource scaling that is required in case of hibernation will now be handled as part of worker reconciliation in extension controllers.
	if gardencorev1beta1helper.HibernationIsEnabled(shoot) {
		logger.V(4).Info("Cluster hibernation is enabled, prober will be removed if present", "namespace", req.Namespace, "name", req.Name)
		r.ProberMgr.Unregister(req.Name)
		return ctrl.Result{}, nil
	}

	// if control plane migration has started for a shoot, then any existing probe should be removed as it is no longer needed.
	if shoot.Status.LastOperation != nil && shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeMigrate {
		logger.V(4).Info("Cluster migration is enabled, prober will be removed if present", "namespace", req.Namespace, "name", req.Name)
		r.ProberMgr.Unregister(req.Name)
		return ctrl.Result{}, nil
	}

	if canStartProber(shoot) {
		logger.V(1).Info("Starting a new probe for cluster if not present", "namespace", req.Namespace, "name", req.Name)
		r.startProber(ctx, req.Name)
	}
	return ctrl.Result{}, nil
}

// getCluster will retrieve the cluster object given the namespace and name Not found is not treated as an error and is handled differently in the caller
func (r *ClusterReconciler) getCluster(ctx context.Context, namespace string, name string) (cluster *gardenerv1alpha1.Cluster, notFound bool, err error) {
	cluster = &gardenerv1alpha1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cluster); err != nil {
		if errors.IsNotFound(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return cluster, false, nil
}

// canStartProber checks if a probe can be registered and started.
// shoot.Status.LastOperation.Type provides an insight into the current state of the cluster. It is important to identify the following cases:
// 1. Cluster has been created successfully => This will ensure that the current state of shoot Kube API Server can be acted upon to decide on scaling operations. If the cluster
// is in the process of creation, then it is possible that the control plane components have not completely come up. If the probe starts prematurely then it could start to scale down resources.
// 2. During control plane migration, the value of shoot.Status.LastOperation.Type will be "Restore" => During this time it is imperative that probe is started early to ensure
// that MCM is scaled down in case connectivity to the Kube API server of the shoot on the destination seed is broken, else it will try and recreate machines.
//
// If the shoot.Status.LastOperation.Type == "Reconcile" then it is assumed that the cluster has been successfully created at-least once and it is safe to start the probe
func canStartProber(shoot *v1beta1.Shoot) bool {
	if shoot.Status.IsHibernated || shoot.Status.LastOperation == nil {
		return false
	}
	if shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeReconcile ||
		shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeRestore ||
		(shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeCreate && shoot.Status.LastOperation.State == v1beta1.LastOperationStateSucceeded) {
		return true
	}
	return false
}

// startProber sets up a new probe against a given key which uniquely identifies the probe.
// Typically, the key in case of a shoot cluster is the shoot namespace
func (r *ClusterReconciler) startProber(ctx context.Context, key string) {
	_, ok := r.ProberMgr.GetProber(key)
	if !ok {
		pLogger := log.FromContext(ctx).WithName(key).WithName("prober")
		deploymentScaler := prober.NewDeploymentScaler(key, r.ProbeConfig, r.Client, r.ScaleGetter, pLogger.WithName("scaler"))
		shootClientCreator := prober.NewShootClientCreator(r.Client)
		p := prober.NewProber(ctx, key, r.ProbeConfig, r.Client, deploymentScaler, shootClientCreator, pLogger)
		r.ProberMgr.Register(*p)
		go p.Run()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gardenerv1alpha1.Cluster{}).
		Complete(r)
}
