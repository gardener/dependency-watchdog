package adjuster

import (
	"context"
	"sync"
	"time"

	adjustapi "github.com/gardener/dependency-watchdog/api/adjuster"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machineutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "adjuster"

var (
	_ adjustapi.Controller = (*defaultController)(nil)
)

type defaultController struct {
	scheme                  *runtime.Scheme
	client                  client.Client
	config                  *adjustapi.Config
	maxConcurrentReconciles int
	state                   state
}

// NewController returns the adjuster implementation of [adjustapi.Controller] initialized with the given [runtime.Scheme]
// , given [client.Client], given adjuster [adjustapi.Config] and maximum number of concurrent Reconciles which can be run.
func NewController(scheme *runtime.Scheme, client client.Client, config *adjustapi.Config, maxConcurrentReconciles int) adjustapi.Controller {
	return new(defaultController{
		scheme:                  scheme,
		client:                  client,
		config:                  config,
		maxConcurrentReconciles: maxConcurrentReconciles,
		state: state{
			mu:                  new(sync.Mutex),
			deploymentStats:     cache.NewExpiring(),
			machineTrackInfos:   cache.NewExpiring(),
			provisionKeyEntries: cache.NewExpiring(),
		},
	})
}

// Reconcile listens to filtered Update events for [machinev1alpha1.Machine] resources and adjusts effective `machine-creation-timeout`s
// on [machinev1alpha1.MachineDeployment]'s.
func (r *defaultController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		m   = new(machinev1alpha1.Machine)
		log = logf.FromContext(ctx)
	)
	log.V(2).Info("Adjuster controller received request")
	if err := r.client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			r.state.clearDataForMachine(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if r.isFistSeenPending(m) {
		return r.reconcileMachineCreate(ctx, m)
	} else if r.isFirstJoin(m) {
		return r.reconcileMachineJoin(ctx, m)
	} else if r.isFirstSeenFailed(m) {
		return r.reconcileMachineFail(ctx, m)
	} else if m.DeletionTimestamp != nil {
		r.state.clearDataForMachine(req.NamespacedName)
	}
	return ctrl.Result{}, nil
}

func (r *defaultController) isFistSeenPending(m *machinev1alpha1.Machine) bool {
	return isPending(m) && !r.state.isRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) isFirstJoin(m *machinev1alpha1.Machine) bool {
	return hasJoined(m) && !r.state.isJoinRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) isFirstSeenFailed(m *machinev1alpha1.Machine) bool {
	return machineutils.IsMachineFailed(m) && !r.state.isFailRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) reconcileMachineCreate(ctx context.Context, m *machinev1alpha1.Machine) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	machineClass, err := r.getMachineClass(ctx, m)
	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if machineClass == nil || err != nil {
		return ctrl.Result{}, nil
	}
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		log.V(2).Info("Cannot reconcileMachineCreate - no 'name' (deployment name) on Machine", "machineName", m.Name)
		return ctrl.Result{}, nil
	}
	pKey := adjustapi.ProvisionKey{
		InstanceType: machineClass.NodeTemplate.InstanceType, // TODO: later this will be available as label on Machine object
		Zone:         m.Spec.NodeTemplateSpec.Labels[corev1.LabelTopologyZone],
	}
	timeout, err := GetEffectiveCreationTimeoutOnMachine(m)
	if err != nil {
		log.Error(err, "Cannot reconcileMachineCreate")
		return ctrl.Result{}, nil
	}
	// TODO: Q: Is this acceptable ttl for map entry?
	ttl := time.Duration(float64(timeout) * *r.config.CreationTimeoutGrowthFactor * 4)
	trackInfo := machineTrackInfo{
		provisionKey:   pKey,
		namespacedName: client.ObjectKeyFromObject(m),
		ttl:            ttl,
	}
	createDuration := time.Since(m.CreationTimestamp.Time)
	deploymentStat := r.state.trackMachineCreate(trackInfo, deploymentName, createDuration)
	log.V(3).Info("Completed reconcileMachineCreate",
		"machineName", m.GetName(),
		"machineDeploymentName", deploymentName,
		"machineClassName", machineClass.Name,
		"provisionKey", pKey,
		"machineDeploymentStat", deploymentStat)
	return ctrl.Result{}, nil
}

func (r *defaultController) reconcileMachineJoin(ctx context.Context, m *machinev1alpha1.Machine) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		log.V(2).Info("Cannot reconcileMachineJoin - no 'name' (deployment name) on Machine", "machineName", m.Name)
		return ctrl.Result{}, nil
	}
	deploymentNamespacedName := types.NamespacedName{Namespace: m.Namespace, Name: deploymentName}
	joinDuration := m.Status.LastOperation.LastUpdateTime.Sub(m.CreationTimestamp.Time)
	trackInfo, deploymentStat, ok := r.state.trackMachineJoin(client.ObjectKeyFromObject(m), deploymentNamespacedName, joinDuration)
	if !ok {
		log.V(2).Info("Cannot trackMachineJoin",
			"machineName", m.Name,
			"machineCreationTimestamp", m.CreationTimestamp,
			"machineJoinDuration", joinDuration)
		return ctrl.Result{}, nil
	}
	var mcd machinev1alpha1.MachineDeployment
	if err := r.client.Get(ctx, deploymentNamespacedName, &mcd); err != nil {
		if apierrors.IsNotFound(err) {
			r.state.clearStateForDeployments(trackInfo.provisionKey, sets.New(deploymentNamespacedName))
			return ctrl.Result{}, nil
		}
		log.Error(err, "Cannot get MachineDeployment in reconcileMachineJoin", "deploymentNamespacedName", deploymentNamespacedName)
		return ctrl.Result{}, err
	}
	watermarkTime := time.Now()
	log.Info("Invoking checkAndUpdateEffectiveCreationTimeout from trackMachineJoin",
		"machineName", m.Name,
		"machineCreationTimestamp", m.CreationTimestamp,
		"machineTrackInfo", trackInfo,
		"provisionKey", trackInfo.provisionKey,
		"watermarkTime", watermarkTime,
		"machineDeploymentStat", deploymentStat)
	return ctrl.Result{}, r.checkAndAdjustEffectiveCreationTimeout(ctx, trackInfo.provisionKey, &mcd, watermarkTime, deploymentStat.joinDuration)
}

func (r *defaultController) reconcileMachineFail(ctx context.Context, m *machinev1alpha1.Machine) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		log.V(2).Info("Cannot reconcileMachineFail - no 'name' (deployment name) on Machine", "machineName", m.Name)
		return ctrl.Result{}, nil
	}
	trackInfo, deploymentStat, ok := r.state.trackMachineFail(client.ObjectKeyFromObject(m), deploymentName)
	if !ok {
		log.V(2).Info("Cannot trackMachineFail",
			"machineName", m.Name,
			"machineDeploymentName", deploymentName,
			"machineCreationTimestamp", m.CreationTimestamp,
			"lastOperation", m.Status.LastOperation,
			"currentStatus", m.Status.CurrentStatus)
		return ctrl.Result{}, nil
	}
	failureThreshold := *r.config.FailureThreshold
	if deploymentStat.failCount < failureThreshold {
		return ctrl.Result{}, nil
	}
	watermarkTime := time.Now()
	log.Info("FailCount has breached configured failureThreshold, invoking growEffectiveCreationTimeoutsOnDeployments",
		"provisionKey", trackInfo.provisionKey,
		"machineDeploymentStat", deploymentStat,
		"watermarkTime", watermarkTime,
		"failureThreshold", failureThreshold)
	return ctrl.Result{}, r.growEffectiveCreationTimeoutsOnDeployments(ctx, trackInfo.provisionKey, watermarkTime)
}

func (r *defaultController) growEffectiveCreationTimeoutsOnDeployments(ctx context.Context, provisionKey adjustapi.ProvisionKey, waterMarkTime time.Time) error {
	log := logf.FromContext(ctx)
	var (
		mcd             = new(machinev1alpha1.MachineDeployment)
		deploymentNames = r.state.getMachineDeploymentNamespacedNames(provisionKey)
		notFoundNames   sets.Set[types.NamespacedName]
	)
	if len(deploymentNames) == 0 {
		log.Info("Cannot growEffectiveCreationTimeoutsOnDeployments since no deploymentNames found for provisionKey", "provisionKey", provisionKey)
	}
	for _, mcdName := range deploymentNames.UnsortedList() {
		if err := r.client.Get(ctx, mcdName, mcd); err != nil {
			if apierrors.IsNotFound(err) {
				notFoundNames.Insert(mcdName)
				continue
			}
			return err
		}
		existingEffectiveTimeout, err := GetEffectiveCreationTimeoutOnMachineDeployment(mcd)
		if err != nil {
			log.Error(err, "Cannot get effective-creation-timeout for MachineDeployment", "machineDeployment", mcdName)
			continue
		}
		newEffectiveTimeout := IncreaseTimeout(existingEffectiveTimeout, *r.config.CreationTimeoutGrowthFactor, r.config.CreationTimeoutMax.Duration)
		if newEffectiveTimeout == 0 {
			continue
		}
		log.Info("Invoking checkAndAdjustEffectiveCreationTimeout for MachineDeployment",
			"machineDeployment", mcd.Name,
			"provisionKey", provisionKey,
			"existingEffectiveTimeout", existingEffectiveTimeout.String(),
			"newEffectiveTimeout", newEffectiveTimeout.String(),
			"waterMarkTime", waterMarkTime)
		err = r.checkAndAdjustEffectiveCreationTimeout(ctx, provisionKey, mcd, waterMarkTime, newEffectiveTimeout)
		if err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}
	if len(notFoundNames) > 0 {
		log.V(3).Info("MachineDeployment(s) were not found - clearing state", "notFoundNames", notFoundNames)
		r.state.clearStateForDeployments(provisionKey, notFoundNames)
	}
	return nil
}

func (r *defaultController) checkAndAdjustEffectiveCreationTimeout(ctx context.Context, provisionKey adjustapi.ProvisionKey, mcd *machinev1alpha1.MachineDeployment, waterMarkTime time.Time, effectiveTimeout time.Duration) error {
	log := logf.FromContext(ctx)
	lastAdjusted, err := GetLastAdjustedEffectiveCreationTimeout(mcd)
	if err != nil {
		log.Error(err, "Cannot get last-adjusted-effective-creation-timeout from MachineDeployment", "machineDeployment", mcd.Name)
		return err
	}
	if waterMarkTime.Sub(lastAdjusted) <= effectiveTimeout {
		log.V(3).Info("Skipping since MachineDeployment's effective-creation-timeout was last adjusted within the effective timeout",
			"machineDeployment", mcd.Name,
			"provisionKey", provisionKey,
			"watermarkTime", waterMarkTime,
			"effectiveTimeout", effectiveTimeout,
			"lastAdjusted", lastAdjusted)
		return nil
	}
	// Check that effectiveTimeout does not go below explicit MCD spec template machine creation timeout (if specified)
	// TODO: Discuss if this is really needed ?
	if mcd.Spec.Template.Spec.MachineCreationTimeout != nil {
		mcdSpecTemplateTimeout := mcd.Spec.Template.Spec.MachineCreationTimeout.Duration
		if mcdSpecTemplateTimeout > effectiveTimeout {
			log.V(3).Info("Skipping adjust since MachineDeployment.Spec.Template.Spec.MachineCreationTimeout is greater than effectiveTimeout",
				"machineDeployment", mcd.Name,
				"provisionKey", provisionKey,
				"watermarkTime", waterMarkTime,
				"specMachineCreationTimeout", mcdSpecTemplateTimeout,
				"effectiveTimeout", effectiveTimeout)
			return nil
		}
	}
	mcdCopy := mcd.DeepCopy()
	metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyEffectiveCreationTimeout, effectiveTimeout.String())
	metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyLastAdjustedEffectiveCreationTimeout, waterMarkTime.Format(time.RFC3339))
	if err = r.client.Update(ctx, mcdCopy); err != nil {
		log.Error(err, "Failed to update MachineDeployment", "machineDeployment", mcd.Name)
		return err
	}
	log.Info("Successfully adjusted effective-creation-timeout for MachineDeployment",
		"machineDeployment", mcd.Name,
		"provisionKey", provisionKey,
		"effectiveTimeout", effectiveTimeout.String(),
		"waterMarkTime", waterMarkTime)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *defaultController) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(
		controllerName,
		mgr,
		controller.Options{
			Reconciler:              r,
			MaxConcurrentReconciles: r.maxConcurrentReconciles},
	)
	if err != nil {
		return err
	}
	return c.Watch(source.Kind[client.Object](mgr.GetCache(),
		&machinev1alpha1.Machine{},
		&handler.EnqueueRequestForObject{}, EventPredicate(c.GetLogger())))
}

func (r *defaultController) getMachineClass(ctx context.Context, m *machinev1alpha1.Machine) (*machinev1alpha1.MachineClass, error) {
	var mcc = new(machinev1alpha1.MachineClass)
	mccName := m.Spec.Class.Name
	err := r.client.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: mccName}, mcc)
	if err != nil {
		return nil, err
	}
	return mcc, nil
}

func (r *defaultController) getMachineDeploymentAndMachineClass(ctx context.Context, m *machinev1alpha1.Machine) (*machinev1alpha1.MachineDeployment, *machinev1alpha1.MachineClass, error) {
	var (
		log = logf.FromContext(ctx)
		mcd = new(machinev1alpha1.MachineDeployment)
		mcc = new(machinev1alpha1.MachineClass)
	)
	mcdName := GetMachineDeploymentName(m)
	if mcdName == "" {
		log.V(5).Info("MachineDeployment 'name' not set as Machine label", "machineName", m.Name)
		return nil, nil, nil
	}
	err := r.client.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: mcdName}, mcd)
	if err != nil {
		return nil, nil, err
	}
	mccName := m.Spec.Class.Name
	err = r.client.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: mccName}, mcd)
	if err != nil {
		return nil, nil, err
	}
	return mcd, mcc, err
}
