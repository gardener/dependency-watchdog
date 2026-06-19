package adjuster

import (
	"context"
	"fmt"
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
	noRequeue ctrl.Result
	_         adjustapi.Controller = (*defaultController)(nil)
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
			mu:                new(sync.Mutex),
			stats:             cache.NewExpiring(),
			freshMachineInfos: cache.NewExpiring(),
		},
	})
}

// Reconcile listens to filtered Update events for [machinev1alpha1.Machine] resources and adjusts effective `machine-creation-timeout`s
// on [machinev1alpha1.MachineDeployment]'s.
func (r *defaultController) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	var (
		m   = new(machinev1alpha1.Machine)
		log = logf.FromContext(ctx)
	)
	log.V(2).Info("Adjuster controller received request.")
	if err = r.client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			r.state.clearDataForMachine(req.NamespacedName)
			err = nil
			return
		}
		return
	}
	if r.isFresh(m) {
		result, err = r.reconcileFresh(ctx, m)
	} else if r.isFirstJoin(m) {
		result = r.reconcileJoin(ctx, m)
	} else if r.isFirstSeenFailed(m) {
		result, err = r.reconcileFailed(ctx, m)
	}
	return
}

func (r *defaultController) isFresh(m *machinev1alpha1.Machine) bool {
	return isNewlyCreated(m) && !r.state.isRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) isFirstJoin(m *machinev1alpha1.Machine) bool {
	return hasJoined(m) && !r.state.isJoinRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) isFirstSeenFailed(m *machinev1alpha1.Machine) bool {
	return machineutils.IsMachineFailed(m) && !r.state.isFailRecorded(client.ObjectKeyFromObject(m))
}
func (r *defaultController) reconcileFresh(ctx context.Context, m *machinev1alpha1.Machine) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx)
	machineClass, err := r.getMachineClass(ctx, m)
	if apierrors.IsNotFound(err) {
		err = nil
		return
	}
	if machineClass == nil || err != nil {
		return
	}
	if err = r.recordCreated(ctx, m, machineClass); err != nil {
		log.V(2).Error(err, "could not record freshly created Machine",
			"machineName", m.Name,
			"machineDeploymentName", GetMachineDeploymentName(m),
			"instanceType", machineClass.NodeTemplate.InstanceType,
			"zone", m.Labels[corev1.LabelTopologyZone])
		return
	}
	return
}

func (r *defaultController) reconcileJoin(ctx context.Context, m *machinev1alpha1.Machine) ctrl.Result {
	log := logf.FromContext(ctx)
	joinDuration := m.Status.LastOperation.LastUpdateTime.Sub(m.CreationTimestamp.Time)
	bInfo, sData, ok := r.state.recordJoin(client.ObjectKeyFromObject(m), joinDuration)
	if !ok {
		log.V(2).Info("cannot record join for Machine",
			"machineName", m.Name,
			"machineCreationTimestamp", m.CreationTimestamp,
			"joinDuration", joinDuration)
		return noRequeue
	}
	log.Info("recorded join for Machine",
		"machineName", m.Name,
		"machineCreationTimestamp", m.CreationTimestamp,
		"machineBasicInfo", bInfo,
		"joinDuration", joinDuration,
		"statData", sData)
	return noRequeue
}

func (r *defaultController) reconcileFailed(ctx context.Context, m *machinev1alpha1.Machine) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx)
	bInfo, sData, ok := r.state.recordFail(client.ObjectKeyFromObject(m))
	if !ok {
		log.V(2).Info("cannot record fail for Machine",
			"machineName", m.Name,
			"machineCreationTimestamp", m.CreationTimestamp,
			"lastOperation", m.Status.LastOperation,
			"currentStatus", m.Status.CurrentStatus)
		return
	}
	log.Info("recorded fail for Machine",
		"machineName", m.Name,
		"machineCreationTimestamp", m.CreationTimestamp,
		"machineBasicInfo", bInfo,
		"statData", sData,
		"lastOperation", m.Status.LastOperation,
		"currentStatus", m.Status.CurrentStatus)
	// now add code here to check failure thresholds and revise the MachineDeployment creation-timeout.
	if sData.failCount < *r.config.FailureThreshold {
		return
	}
	log.Info("statData.failCount breached configured FailureThreshold",
		"provisionKey", bInfo.provisionKey,
		"statData", sData,
		"config.FailureThreshold", *r.config.FailureThreshold)
	err = r.adjustEffectiveCreationTimeouts(ctx, bInfo.provisionKey, time.Now())
	return
}

func (r *defaultController) adjustEffectiveCreationTimeouts(ctx context.Context, key adjustapi.ProvisionKey, markTime time.Time) error {
	log := logf.FromContext(ctx)
	//adjusted := sets.New[types.NamespacedName]()
	var (
		mcd             = new(machinev1alpha1.MachineDeployment)
		mcdCopy         *machinev1alpha1.MachineDeployment
		deploymentNames = r.state.getMachineDeploymentNames(key)
		notFoundNames   sets.Set[types.NamespacedName]
	)

	for _, mcdName := range deploymentNames.UnsortedList() {
		if err := r.client.Get(ctx, mcdName, mcd); err != nil {
			if apierrors.IsNotFound(err) {
				notFoundNames.Insert(mcdName)
				continue
			}
			return err
		}
		effectiveTimeout, err := GetEffectiveMachineCreationTimeoutFromRuntimeObject(mcd)
		if err != nil {
			log.Error(err, "cannot get effective-creation-timeout from MachineDeployment", "machineDeployment", mcdName)
			continue
		}
		lastAdjusted, err := GetLastAdjustedEffectiveCreationTimeout(mcd)
		if err != nil {
			log.Error(err, "cannot get last-adjusted-effective-creation-timeout from MachineDeployment", "machineDeployment", mcdName)
			continue
		}
		if markTime.Sub(lastAdjusted) <= effectiveTimeout {
			log.V(3).Info("Skipping adjustment since MachineDeployment was last adjusted within the effective timeout",
				"machineDeployment", mcdName,
				"watermarkTime", markTime,
				"lastAdjusted", lastAdjusted,
				"effectiveTimeout", effectiveTimeout)
			continue
		}
		adjustedTimeout := AdjustTimeout(effectiveTimeout, *r.config.CreationTimeoutGrowthFactor, r.config.CreationTimeoutMax.Duration)
		if adjustedTimeout == 0 {
			continue
		}
		mcdCopy = mcd.DeepCopy()
		metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyEffectiveCreationTimeout, adjustedTimeout.String())
		metav1.SetMetaDataAnnotation(&mcdCopy.ObjectMeta, adjustapi.AnnotationKeyLastAdjustedEffectiveCreationTimeout, markTime.Format(time.RFC3339))
		err = r.client.Update(ctx, mcdCopy)
		if err != nil {
			if apierrors.IsNotFound(err) {
				notFoundNames.Insert(mcdName)
				continue
			}
			return err
		}
		log.Info("Adjusted effective-creation-timeout for MachineDeployment.",
			"machineDeployment", mcdName,
			"adjustedTimeout", adjustedTimeout,
			"previousTimeout", effectiveTimeout,
			"markTime", markTime)
	}
	if len(notFoundNames) > 0 {
		log.V(3).Info("MachineDeployment(s) were not found. Removing from relatedDeployments", "notFoundNames", notFoundNames)
		r.state.removeFromRelatedDeployments(key, notFoundNames.UnsortedList()...)
	}
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

func (r *defaultController) recordCreated(ctx context.Context, m *machinev1alpha1.Machine, mcc *machinev1alpha1.MachineClass) error {
	log := logf.FromContext(ctx)
	deploymentName := GetMachineDeploymentName(m)
	if deploymentName == "" {
		return fmt.Errorf("%w: no 'name' (deployment name) annotation on Machine %q", ErrCannotRecordFreshMachine, m.Name)
	}
	pKey := adjustapi.ProvisionKey{
		InstanceType: mcc.NodeTemplate.InstanceType,
		Zone:         m.Spec.NodeTemplateSpec.Labels[corev1.LabelTopologyZone],
	}
	timeout, err := GetEffectiveCreationTimeoutOnMachine(m)
	if err != nil {
		return err
	}
	expiry := time.Duration(float32(timeout) * *r.config.CreationTimeoutGrowthFactor)
	basicInfo := machineBasicInfo{
		expiry:       expiry,
		provisionKey: pKey,
	}
	createDuration := time.Now().Sub(m.CreationTimestamp.Time)
	updatedData := r.state.recordFresh(client.ObjectKeyFromObject(m), basicInfo, createDuration)
	log.V(3).Info("created fresh machine record.", "recordKey", pKey, "recordData", updatedData)
	return nil
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
		log.V(5).Info("MachineDeployment Name not set for Machine.", "machineName", m.Name)
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
