// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"fmt"
	"github.com/gardener/dependency-watchdog/internal/util"

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
	// Scheme is the controller-runtime scheme used to initialize the controller manager and to validate the probe config
	Scheme *runtime.Scheme
	// ProberMgr is interface to manage lifecycle of probers.
	ProberMgr prober.Manager
	// ScaleGetter is used to produce a ScaleInterface
	ScaleGetter scale.ScalesGetter
	// DefaultProbeConfig is the seed level config inherited by all shoots whose control plane is hosted in the seed. The default config is used
	// when the shoot's spec.Kubernetes.KubeControllerManager.NodeMonitorGracePeriod is not set. If it is set, then a new config is generated from
	// the default config with the updated KCMNodeMonitorGraceDuration.
	DefaultProbeConfig *papi.Config
	// MaxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1.
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

	if shouldStopProber(shoot, log) {
		if r.ProberMgr.Unregister(req.Name) {
			log.Info("Existing prober has been removed")
		}
		return ctrl.Result{}, nil
	}

	if canStartProber(shoot, log) {
		r.startProber(ctx, shoot, log)
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

// startProber sets up a new probe against a given key which uniquely identifies the probe.
// Typically, the key in case of a shoot cluster is the shoot namespace
func (r *Reconciler) startProber(ctx context.Context, shoot *v1beta1.Shoot, logger logr.Logger) {
	workerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	existingProber, ok := r.ProberMgr.GetProber(shoot.Name)
	if !ok {
		r.createAndRunProber(ctx, shoot, workerNodeConditions, logger)
	} else {
		if existingProber.AreWorkerNodeConditionsStale(workerNodeConditions) {
			logger.Info("restarting prober due to change in node conditions for workers")
			_ = r.ProberMgr.Unregister(shoot.Name)
			r.createAndRunProber(ctx, shoot, workerNodeConditions, logger)
		}
	}
}

func (r *Reconciler) createAndRunProber(ctx context.Context, shoot *v1beta1.Shoot, workerNodeConditions map[string][]string, logger logr.Logger) {
	probeConfig := r.getEffectiveProbeConfig(shoot, logger)
	deploymentScaler := scaler.NewScaler(shoot.Name, probeConfig.DependentResourceInfos, r.Client, r.ScaleGetter, logger)
	shootClientCreator := prober.NewShootClientCreator(r.Client)
	p := prober.NewProber(ctx, shoot.Name, probeConfig, workerNodeConditions, deploymentScaler, shootClientCreator, logger)
	r.ProberMgr.Register(*p)
	logger.Info("Starting a new prober")
	go p.Run()
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
		probeConfig.KCMNodeMonitorGraceDuration = kcmConfig.NodeMonitorGracePeriod
	}
	return &probeConfig
}

func shouldStopProber(shoot *v1beta1.Shoot, logger logr.Logger) bool {
	// If shoot is marked for deletion then any existing probes will be unregistered
	if shoot.DeletionTimestamp != nil {
		logger.Info("Cluster has been marked for deletion, existing prober if any will be removed")
		return true
	}

	// if hibernation is enabled then we will remove any existing prober. Any resource scaling that is required in case of hibernation will now be handled as part of worker reconciliation in extension controllers.
	if v1beta1helper.HibernationIsEnabled(shoot) {
		logger.Info("Cluster hibernation is enabled, existing prober if any will be removed")
		return true
	}

	// if control plane migration has started for a shoot, then any existing probe should be removed as it is no longer needed.
	if shoot.Status.LastOperation != nil && shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeMigrate {
		logger.Info("Cluster migration is enabled, existing prober if any will be removed")
		return true
	}

	// if a shoot is created without any workers (this can only happen for control-plane-as-a-service use case), then any existing probe should be removed as it is no longer needed.
	if len(shoot.Spec.Provider.Workers) == 0 {
		logger.Info("Cluster does not have any workers, existing prober if any will be removed")
		return true
	}
	return false
}

// canStartProber checks if a probe can be registered and started.
// shoot.Status.LastOperation.Type provides an insight into the current state of the cluster. It is important to identify the following cases:
// 1. Cluster has been created successfully => This will ensure that the current state of shoot Kube API Server can be acted upon to decide on scaling operations. If the cluster
// is in the process of creation, then it is possible that the control plane components have not completely come up. If the probe starts prematurely then it could start to scale down resources.
// 2. During control plane migration, the value of shoot.Status.LastOperation.Type will be "Restore" => During this time it is imperative that probe is started early to ensure
// that MCM is scaled down in case connectivity to the Kube API server of the shoot on the destination seed is broken, else it will try and recreate machines.
// If the shoot.Status.LastOperation.Type == "Reconcile" then it is assumed that the cluster has been successfully created at-least once, and it is safe to start the probe.
func canStartProber(shoot *v1beta1.Shoot, logger logr.Logger) bool {
	if !v1beta1helper.HibernationIsEnabled(shoot) && shoot.Status.IsHibernated {
		logger.Info("Cannot start probe. Cluster is waking up from hibernation")
		return false
	}
	if shoot.Status.LastOperation == nil {
		logger.Info("Cannot start probe. Cluster is creation phase")
		return false
	}
	if shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeReconcile ||
		(shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeRestore && shoot.Status.LastOperation.State == v1beta1.LastOperationStateSucceeded) ||
		(shoot.Status.LastOperation.Type == v1beta1.LastOperationTypeCreate && shoot.Status.LastOperation.State == v1beta1.LastOperationStateSucceeded) {
		return true
	}
	logger.Info("Cannot start probe. Cluster is either in migration or in creation phase")
	return false
}
