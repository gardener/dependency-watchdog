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

package cluster

import (
	"context"
	"fmt"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"

	"github.com/gardener/dependency-watchdog/internal/prober"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	v1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const controllerName = "cluster"

// Reconciler reconciles a Cluster object
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	ProberMgr               prober.Manager
	ScaleGetter             scale.ScalesGetter
	DefaultProbeConfig      *papi.Config
	MaxConcurrentReconciles int
}

//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=gardener.cloud,resources=clusters/status,verbs=get

// Reconcile listens to create/update/delete events for `Cluster` resources and
// manages probes for the shoot control namespace for these clusters by looking at the cluster state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	cluster, notFound, err := r.getCluster(ctx, req.Namespace, req.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get cluster resource: %w", err)
	}
	// If the cluster is not found then any existing probes if present will be unregistered
	if notFound {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Cluster not found, existing prober has been removed")
		}
		return ctrl.Result{}, nil
	}

	shoot, err := extensionscontroller.ShootFromCluster(cluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error extracting shoot from cluster: %w", err)
	}

	// If shoot is marked for deletion then any existing probes will be unregistered
	if shoot.DeletionTimestamp != nil {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Cluster has been marked for deletion, existing prober has been removed")
		}
		return ctrl.Result{}, nil
	}

	// if hibernation is enabled then we will remove any existing prober. Any resource scaling that is required in case of hibernation will now be handled as part of worker reconciliation in extension controllers.
	if v1beta1helper.HibernationIsEnabled(shoot) {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Cluster hibernation is enabled, existing prober has been removed")
		}
		return ctrl.Result{}, nil
	}

	// if control plane migration has started for a shoot, then any existing probe should be removed as it is no longer needed.
	if shoot.Status.LastOperation != nil && shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeMigrate {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Cluster migration is enabled, existing prober has been removed")
		}
		return ctrl.Result{}, nil
	}

	// if a shoot is created without any workers (this can only happen for control-plane-as-a-service use case), then if there is a probe registered then
	// unregister the probe and return early. If there is no existing probe registered then return early.
	if len(shoot.Spec.Provider.Workers) == 0 {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Cluster does not have any workers. An existing probe has been removed")
		} else {
			log.Info("Cluster does not have any workers. No probe will be created")
		}
		return ctrl.Result{}, nil
	}

	if canStartProber(shoot) {
		r.startProber(ctx, shoot, log, req.Name)
	}
	return ctrl.Result{}, nil
}

// getCluster will retrieve the cluster object given the namespace and name Not found is not treated as an error and is handled differently in the caller
func (r *Reconciler) getCluster(ctx context.Context, namespace string, name string) (cluster *extensionsv1alpha1.Cluster, notFound bool, err error) {
	cluster = &extensionsv1alpha1.Cluster{}
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
// If the shoot.Status.LastOperation.Type == "Reconcile" then it is assumed that the cluster has been successfully created at-least once, and it is safe to start the probe.
func canStartProber(shoot *v1beta1.Shoot) bool {
	if shoot.Status.IsHibernated || shoot.Status.LastOperation == nil {
		return false
	}
	if shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeReconcile ||
		(shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeRestore && shoot.Status.LastOperation.State == v1beta1.LastOperationStateSucceeded) ||
		(shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeCreate && shoot.Status.LastOperation.State == v1beta1.LastOperationStateSucceeded) {
		return true
	}
	return false
}

// startProber sets up a new probe against a given key which uniquely identifies the probe.
// Typically, the key in case of a shoot cluster is the shoot namespace
func (r *Reconciler) startProber(ctx context.Context, shoot *v1beta1.Shoot, logger logr.Logger, key string) {
	_, ok := r.ProberMgr.GetProber(key)
	if !ok {
		probeConfig := r.getEffectiveProbeConfig(shoot, logger)
		deploymentScaler := scaler.NewScaler(key, r.DefaultProbeConfig, r.Client, r.ScaleGetter, logger)
		shootClientCreator := prober.NewShootClientCreator(r.Client)
		p := prober.NewProber(ctx, key, probeConfig, deploymentScaler, shootClientCreator, logger)
		r.ProberMgr.Register(*p)
		logger.Info("Starting a new prober")
		go p.Run()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(
		controllerName,
		mgr,
		controller.Options{
			Reconciler:              r,
			MaxConcurrentReconciles: r.MaxConcurrentReconciles},
	)
	if err != nil {
		return err
	}
	return c.Watch(source.Kind(mgr.GetCache(), &extensionsv1alpha1.Cluster{}), &handler.EnqueueRequestForObject{}, workerLessShoot(c.GetLogger()))
}

// getEffectiveProbeConfig returns the updated probe config after checking the shoot KCM configuration for NodeMonitorGracePeriod.
// If NodeMonitorGracePeriod is not set in the shoot, then the KCMNodeMonitorGraceDuration defined in the configmap of probe config will be used
func (r *Reconciler) getEffectiveProbeConfig(shoot *v1beta1.Shoot, logger logr.Logger) *papi.Config {
	probeConfig := *r.DefaultProbeConfig
	kcmConfig := shoot.Spec.Kubernetes.KubeControllerManager
	if kcmConfig != nil && kcmConfig.NodeMonitorGracePeriod != nil {
		logger.Info("Using the NodeMonitorGracePeriod set in the shoot as KCMNodeMonitorGraceDuration in the probe config", "nodeMonitorGraceDuration", *kcmConfig.NodeMonitorGracePeriod)
		probeConfig.KCMNodeMonitorGraceDuration = *kcmConfig.NodeMonitorGracePeriod
	}
	return &probeConfig
}
