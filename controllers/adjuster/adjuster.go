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

// Reconciler is the controller implementation to track machine joins and adjust the effective-creation-timeout.
type Reconciler struct {
	scheme                  *runtime.Scheme
	client                  client.Client
	config                  *adjustapi.Config
	maxConcurrentReconciles int
	state                   state
}

// NewController returns the default adjuster controller initialized with the given [runtime.Scheme], given
// [client.Client], given adjuster [adjustapi.Config] and maximum number of concurrent Reconciles which can be run.
func NewController(scheme *runtime.Scheme, client client.Client, config *adjustapi.Config, maxConcurrentReconciles int) *Reconciler {
	return &Reconciler{
		scheme:                  scheme,
		client:                  client,
		config:                  config,
		maxConcurrentReconciles: maxConcurrentReconciles,
		state: state{
			mu:                new(sync.Mutex),
			deploymentStats:   cache.NewExpiring(),
			machineTrackInfos: cache.NewExpiring(),
			provisionKeyStats: cache.NewExpiring(),
		},
	}
}

// Reconcile listens to filtered Update events for [machinev1alpha1.Machine] resources and adjusts effective `machine-creation-timeout`s
// on [machinev1alpha1.MachineDeployment]'s.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		m   = new(machinev1alpha1.Machine)
		log = logf.FromContext(ctx)
	)
	log.V(2).Info("Adjuster controller received request")
	if err := r.client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			r.state.untrackMachine(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if r.isFirstJoin(m) {
		return r.reconcileMachineJoin(ctx, m)
	} else if r.isFirstSeenFailed(m) {
		return r.reconcileMachineFail(ctx, m)
	} else if m.DeletionTimestamp != nil {
		r.state.untrackMachine(req.NamespacedName)
	}
	return ctrl.Result{}, nil
}
func (r *Reconciler) isFirstJoin(m *machinev1alpha1.Machine) bool {
	return hasJoined(m) && !r.state.isJoinRecorded(client.ObjectKeyFromObject(m))
}
func (r *Reconciler) isFirstSeenFailed(m *machinev1alpha1.Machine) bool {
	return machineutils.IsMachineFailed(m) && !r.state.isFailRecorded(client.ObjectKeyFromObject(m))
}

func (r *Reconciler) reconcileMachineJoin(ctx context.Context, m *machinev1alpha1.Machine) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	provisionKey, err := r.getProvisionKey(ctx, m)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		log.V(2).Info("Cannot reconcileMachineJoin - no 'name' (deployment name) on Machine", "machineName", m.Name)
		return ctrl.Result{}, nil
	}
	timeout, err := GetEffectiveCreationTimeoutOnMachine(m)
	if err != nil {
		log.Error(err, "Cannot reconcileMachineJoin")
		return ctrl.Result{}, nil
	}
	ttl := time.Duration(float64(timeout) * *r.config.CreationTimeoutGrowthFactor)
	joinDuration := m.Status.LastOperation.LastUpdateTime.Sub(m.CreationTimestamp.Time)
	machineNsName := client.ObjectKeyFromObject(m)
	deploymentStat, _ := r.state.recordMachineJoin(machineNsName, deploymentName, provisionKey, joinDuration, ttl)
	log.V(3).Info("Successful recordMachineJoin",
		"machineName", m.GetName(),
		"machineCreationTimestamp", m.CreationTimestamp,
		"joinDuration", joinDuration,
		"machineDeploymentName", deploymentName,
		"provisionKey", provisionKey,
		"machineDeploymentStat", deploymentStat)
	deploymentNamespacedName := types.NamespacedName{Namespace: m.Namespace, Name: deploymentName}
	var mcd machinev1alpha1.MachineDeployment
	if err := r.client.Get(ctx, deploymentNamespacedName, &mcd); err != nil {
		if apierrors.IsNotFound(err) {
			r.state.clearDeploymentStats(provisionKey, sets.New(deploymentNamespacedName))
			return ctrl.Result{}, nil
		}
		log.Error(err, "Cannot get MachineDeployment in reconcileMachineJoin", "deploymentNamespacedName", deploymentNamespacedName)
		return ctrl.Result{}, err
	}
	watermarkTime := time.Now()
	log.Info("Invoking checkAndUpdateEffectiveCreationTimeout from reconcileMachineJoin",
		"machineName", m.Name,
		"machineCreationTimestamp", m.CreationTimestamp,
		"machineNsName", machineNsName,
		"provisionKey", provisionKey,
		"watermarkTime", watermarkTime,
		"machineDeploymentStat", deploymentStat)
	return ctrl.Result{}, r.checkAndAdjustEffectiveCreationTimeout(ctx, provisionKey, &mcd, watermarkTime, deploymentStat.maxJoinDuration)
}

func (r *Reconciler) reconcileMachineFail(ctx context.Context, m *machinev1alpha1.Machine) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	provisionKey, err := r.getProvisionKey(ctx, m)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		log.V(2).Info("Cannot reconcileMachineFail - no 'name' (deployment name) on Machine", "machineName", m.Name)
		return ctrl.Result{}, nil
	}
	timeout, err := GetEffectiveCreationTimeoutOnMachine(m)
	if err != nil {
		log.Error(err, "Cannot reconcileMachineFail")
		return ctrl.Result{}, nil
	}
	ttl := time.Duration(float64(timeout) * *r.config.CreationTimeoutGrowthFactor)
	machineNsName := client.ObjectKeyFromObject(m)
	deploymentStat, provisionKeyStat := r.state.recordMachineFail(machineNsName, deploymentName, provisionKey, ttl)
	log.V(3).Info("Successful recordMachineFail",
		"machineName", m.GetName(),
		"machineCreationTimestamp", m.CreationTimestamp,
		"machineDeploymentName", deploymentName,
		"provisionKey", provisionKey,
		"machineDeploymentStat", deploymentStat)
	if !provisionKeyStat.HasBreached(*r.config.MachineFailureThresholdMin, *r.config.MachineFailureThresholdFraction) {
		return ctrl.Result{}, nil
	}
	watermarkTime := time.Now()
	log.Info("MachineFailureThresholdFraction Breached, invoking growEffectiveCreationTimeoutsOnDeployments",
		"provisionKey", provisionKey,
		"machineProvisionKeyStat", provisionKeyStat,
		"machineDeploymentStat", deploymentStat,
		"watermarkTime", watermarkTime,
		"machineFailureThresholdMin", *r.config.MachineFailureThresholdMin,
		"machineFailureThresholdFraction", *r.config.MachineFailureThresholdFraction)
	return ctrl.Result{}, r.growEffectiveCreationTimeoutsOnDeployments(ctx, provisionKey, watermarkTime)
}

func (r *Reconciler) getProvisionKey(ctx context.Context, m *machinev1alpha1.Machine) (provisionKey adjustapi.MachineProvisionKey, err error) {
	machineClass, err := r.getMachineClass(ctx, m)
	if err != nil {
		return
	}
	provisionKey = adjustapi.MachineProvisionKey{
		InstanceType: machineClass.NodeTemplate.InstanceType, // TODO: later this will be available as label on Machine object
		Zone:         m.Spec.NodeTemplateSpec.Labels[corev1.LabelTopologyZone],
	}
	return
}

func (r *Reconciler) growEffectiveCreationTimeoutsOnDeployments(ctx context.Context, provisionKey adjustapi.MachineProvisionKey, waterMarkTime time.Time) error {
	log := logf.FromContext(ctx)
	var (
		mcd             = new(machinev1alpha1.MachineDeployment)
		deploymentNames = r.state.getMachineDeploymentNamespacedNames(provisionKey)
		notFoundNames   = sets.New[types.NamespacedName]()
	)
	if len(deploymentNames) == 0 {
		log.Info("Cannot growEffectiveCreationTimeoutsOnDeployments since no deploymentNames found for provisionKey", "provisionKey", provisionKey)
		return nil
	}
	for _, mcdName := range deploymentNames.UnsortedList() {
		if err := r.client.Get(ctx, mcdName, mcd); err != nil {
			if apierrors.IsNotFound(err) {
				notFoundNames.Insert(mcdName)
				continue
			}
			return err
		}
		currentEffectiveTimeout, err := GetEffectiveCreationTimeoutOnMachineDeployment(mcd)
		if err != nil {
			log.Error(err, "Cannot get effective-creation-timeout for MachineDeployment", "machineDeployment", mcdName)
			continue
		}
		newEffectiveTimeout := IncreaseTimeout(currentEffectiveTimeout, *r.config.CreationTimeoutGrowthFactor, r.config.CreationTimeoutMax.Duration)
		if newEffectiveTimeout == 0 {
			log.Info("Skipping MachineDeployment: effective-creation-timeout already at or above maximum",
				"machineDeployment", mcdName,
				"provisionKey", provisionKey,
				"currentEffectiveTimeout", currentEffectiveTimeout,
				"creationTimeoutMax", r.config.CreationTimeoutMax.Duration)
			continue
		}
		log.Info("Invoking checkAndAdjustEffectiveCreationTimeout for MachineDeployment",
			"machineDeployment", mcd.Name,
			"provisionKey", provisionKey,
			"currentEffectiveTimeout", currentEffectiveTimeout.String(),
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
		r.state.clearDeploymentStats(provisionKey, notFoundNames)
	}
	return nil
}

func (r *Reconciler) checkAndAdjustEffectiveCreationTimeout(ctx context.Context, provisionKey adjustapi.MachineProvisionKey, mcd *machinev1alpha1.MachineDeployment, waterMarkTime time.Time, newEffectiveTimeout time.Duration) error {
	log := logf.FromContext(ctx)
	currentEffectiveTimeout, err := GetEffectiveCreationTimeoutOnMachineDeployment(mcd)
	if err != nil {
		log.Error(err, "Cannot get effective-creation-timeout from MachineDeployment", "machineDeployment", mcd.Name)
		return err
	}
	lastAdjusted, err := GetLastAdjustedEffectiveCreationTimeout(mcd)
	if err != nil {
		log.Error(err, "Cannot get last-adjusted-effective-creation-timeout from MachineDeployment", "machineDeployment", mcd.Name)
		return err
	}
	// Skip if the annotation was last written within the current effectiveTimeout window.
	// This acts as a cooldown: machines already spawning under the current effective timeout are still
	// within their allowed creation window, so changing the timeout again is premature.
	if waterMarkTime.Sub(lastAdjusted) <= currentEffectiveTimeout {
		log.V(4).Info("Skipping since MachineDeployment's effective-creation-timeout was last adjusted within the current effective timeout",
			"machineDeployment", mcd.Name,
			"provisionKey", provisionKey,
			"watermarkTime", waterMarkTime,
			"newEffectiveTimeout", newEffectiveTimeout,
			"currentEffectiveTimeout", currentEffectiveTimeout,
			"lastAdjusted", lastAdjusted)
		return nil
	}
	// Check that effectiveTimeout does not go below explicit MCD spec template machine creation timeout (if specified)
	if mcd.Spec.Template.Spec.MachineConfiguration != nil && mcd.Spec.Template.Spec.MachineCreationTimeout != nil {
		mcdSpecTemplateTimeout := mcd.Spec.Template.Spec.MachineCreationTimeout.Duration
		if mcdSpecTemplateTimeout > newEffectiveTimeout {
			log.V(4).Info("Skipping adjust since MachineDeployment.Spec.Template.Spec.MachineCreationTimeout is greater than effectiveTimeout",
				"machineDeployment", mcd.Name,
				"provisionKey", provisionKey,
				"watermarkTime", waterMarkTime,
				"specMachineCreationTimeout", mcdSpecTemplateTimeout,
				"currentEffectiveTimeout", currentEffectiveTimeout,
				"newEffectiveTimeout", newEffectiveTimeout)
			return nil
		}
	}
	deploymentStat, ok := r.state.getMachineDeploymentStat(client.ObjectKeyFromObject(mcd))
	if !ok {
		return nil
	}
	hasGrown := newEffectiveTimeout > currentEffectiveTimeout
	if deploymentStat.joinStreakCount > 0 && hasGrown {
		// Do not increase adjustapi.AnnotationKeyEffectiveCreationTimeout if Machines are joining for the MachineDeployment.
		// Shrinks bypass this guard so that a join can reset the annotation down to the observed join duration.
		log.V(3).Info("Skipping adjust since machines are joining for the MachineDeployment and effectiveTimeout is not a reduction",
			"machineDeployment", mcd.Name,
			"machineDeploymentStat", deploymentStat,
			"currentEffectiveTimeout", currentEffectiveTimeout,
			"newEffectiveTimeout", newEffectiveTimeout,
			"watermarkTime", waterMarkTime)
		return nil
	}
	mcdCopy := mcd.DeepCopy()
	metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyEffectiveCreationTimeout, newEffectiveTimeout.String())
	metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyEffectiveCreationTimeoutLastAdjustedAt, waterMarkTime.Format(time.RFC3339))
	if err = r.client.Update(ctx, mcdCopy); err != nil {
		log.Error(err, "Failed to update MachineDeployment", "machineDeployment", mcd.Name)
		return err
	}
	log.Info("Successfully adjusted effective-creation-timeout for MachineDeployment",
		"machineDeployment", mcd.Name,
		"provisionKey", provisionKey,
		"previousEffectiveTimeout", currentEffectiveTimeout,
		"newEffectiveTimeout", newEffectiveTimeout,
		"waterMarkTime", waterMarkTime)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
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

func (r *Reconciler) getMachineClass(ctx context.Context, m *machinev1alpha1.Machine) (*machinev1alpha1.MachineClass, error) {
	var mcc = new(machinev1alpha1.MachineClass)
	mccName := m.Spec.Class.Name
	err := r.client.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: mccName}, mcc)
	if err != nil {
		return nil, err
	}
	return mcc, nil
}
