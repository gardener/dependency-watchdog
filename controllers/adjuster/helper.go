package adjuster

import (
	"strings"
	"time"

	adjustapi "github.com/gardener/dependency-watchdog/api/adjuster"
	"github.com/gardener/dependency-watchdog/internal/util"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// LoadConfig reads the adjuster configuration from a file if specified, unmarshalls into instance of
// [adjustapi.Config], fills default values for missing config fields using [FillConfigDefaults] and returns
// populated object or an error.
func LoadConfig(configPath string) (config *adjustapi.Config, err error) {
	if configPath != "" {
		config, err = util.ReadAndUnmarshall[adjustapi.Config](configPath)
		if err != nil {
			return nil, err
		}
	} else {
		config = &adjustapi.Config{}
	}
	FillConfigDefaults(config)
	err = ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func ValidateConfig(c *adjustapi.Config) error {
	v := new(util.Validator)
	if c.CreationTimeoutMax != nil {
		v.MustNotBeZeroDuration("CreationTimeoutMax", *c.CreationTimeoutMax)
	}
	if c.CreationTimeoutGrowthFactor != nil {
		v.MustBeGreater("CreationTimeoutGrowthFactor", *c.CreationTimeoutGrowthFactor, 1.0)
	}
	if v.Error != nil {
		return v.Error
	}
	return nil
}

// FillConfigDefaults fills default values for missing config entries inside the given [adjustapi.Config]
func FillConfigDefaults(c *adjustapi.Config) {
	c.CreationTimeoutGrowthFactor = util.GetValOrDefault(c.CreationTimeoutGrowthFactor, 2.0)
	c.CreationTimeoutMax = util.GetValOrDefault(c.CreationTimeoutMax, metav1.Duration{Duration: adjustapi.DefaultCreationTimeoutMax})
	c.FailureThreshold = util.GetValOrDefault(c.FailureThreshold, adjustapi.DefaultFailureThreshold)
}

// EventPredicate creates controller runtime [predicate.Predicate] for [machinev1alpha1.Machine] Updated event that satisfy [CanAcceptForAdjusterReconcile]
func EventPredicate(logger logr.Logger) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return CanAcceptForAdjusterReconcile(logger, updateEvent.ObjectOld, updateEvent.ObjectNew)
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

// CanAcceptForAdjusterReconcile returns true if
//   - Machine moved from PENDING->FAILED phase
//   - Machine moved from PENDING->RUNNING phase
func CanAcceptForAdjusterReconcile(log logr.Logger, objOld, objNew client.Object) (accept bool) {
	machineOld, ok := objOld.(*machinev1alpha1.Machine)
	if !ok {
		return
	}
	machineNew, ok := objNew.(*machinev1alpha1.Machine)
	if !ok {
		return
	}
	oldPhase := machineOld.Status.CurrentStatus.Phase
	currPhase := machineNew.Status.CurrentStatus.Phase

	// TODO: BEGIN REMOVE ME {
	if !strings.Contains(machineNew.Name, "i034796") {
		return
	}
	// TODO: END REMOVE ME

	if oldPhase == "" && currPhase == machinev1alpha1.MachinePending {
		accept = true
	}
	if (oldPhase == "" || oldPhase == machinev1alpha1.MachinePending) &&
		(currPhase == machinev1alpha1.MachineAvailable || currPhase == machinev1alpha1.MachineRunning) {
		accept = true
	}
	if oldPhase == machinev1alpha1.MachinePending && currPhase == machinev1alpha1.MachineFailed {
		accept = true
	}
	//if currPhase == machinev1alpha1.MachineAvailable {
	//	// sometime times Pending->Running event is missed and hence we need to check Available->Available update events too
	//	// for Machines that have successfully joined cluster.
	//	// I am not able to reproduce this currently.
	//	accept = true
	//}
	if accept {
		log.V(2).Info("Machine accepted for adjuster reconcile",
			"name", machineNew.Name,
			"oldPhase", oldPhase,
			"currPhase", currPhase,
			"lastOperation", machineNew.Status.LastOperation)
	} else {
		log.V(3).Info("Machine skipped for adjuster reconcile", "name", machineNew.Name, "oldPhase", oldPhase, "currPhase", currPhase, "status.lastOperation", machineNew.Status.LastOperation)
	}
	return
}

// GetEffectiveCreationTimeoutOnMachine gets the effective creation timeout for this Machine object, first checking
// the [adjustapi.AnnotationKeyEffectiveCreationTimeout], then falling back to machine spec and then falling back
// to [adjustapi.StandardCreationTimeout].
func GetEffectiveCreationTimeoutOnMachine(m *machinev1alpha1.Machine) (duration time.Duration, err error) {
	duration, err = GetEffectiveMachineCreationTimeoutFromRuntimeObject(m)
	if err != nil || duration != 0 {
		return
	}
	if m.Spec.MachineCreationTimeout != nil {
		duration = m.Spec.MachineCreationTimeout.Duration
		return
	}
	duration = adjustapi.StandardCreationTimeout
	return
}

// GetEffectiveCreationTimeoutOnMachineDeployment gets the effective creation timeout for this MachineDeployment object, first checking
// the [adjustapi.AnnotationKeyEffectiveCreationTimeout], then falling back to machine deployment spec template and then falling back
// to [adjustapi.StandardCreationTimeout]
func GetEffectiveCreationTimeoutOnMachineDeployment(mcd *machinev1alpha1.MachineDeployment) (duration time.Duration, err error) {
	duration, err = GetEffectiveMachineCreationTimeoutFromRuntimeObject(mcd)
	if err != nil || duration != 0 {
		return
	}
	if mcd.Spec.Template.Spec.MachineCreationTimeout != nil {
		duration = mcd.Spec.Template.Spec.MachineCreationTimeout.Duration
		return
	}
	duration = adjustapi.StandardCreationTimeout
	return
}

// GetLastAdjustedEffectiveCreationTimeout returns the parsed value of the annotation [adjustapi.AnnotationKeyLastAdjustedEffectiveCreationTimeout].
// This represents the time at which the [adjustapi.AnnotationKeyEffectiveCreationTimeout] was last set on this machine.
// Returns zero if not set
func GetLastAdjustedEffectiveCreationTimeout(mcd *machinev1alpha1.MachineDeployment) (lastAdjusted time.Time, err error) {
	metaObject, err := meta.Accessor(mcd)
	if err != nil {
		return
	}
	lastAdjustedStr, ok := metaObject.GetAnnotations()[adjustapi.AnnotationKeyLastAdjustedEffectiveCreationTimeout]
	if !ok {
		return
	}
	lastAdjusted, err = time.Parse(time.RFC3339, lastAdjustedStr)
	return
}

// GetEffectiveMachineCreationTimeoutFromRuntimeObject gets the value of the annotation [v1alpha1.AnnotationKeyMachineEffectiveCreationTimeout]
// as a [time.Duration] if present. Returns zero value if not present or an error if an error was encountered.
func GetEffectiveMachineCreationTimeoutFromRuntimeObject(object runtime.Object) (duration time.Duration, err error) {
	metaObject, err := meta.Accessor(object)
	if err != nil {
		return
	}
	effectiveMachineCreationTimeoutStr, ok := metaObject.GetAnnotations()[adjustapi.AnnotationKeyEffectiveCreationTimeout]
	if !ok {
		return
	}
	duration, err = time.ParseDuration(effectiveMachineCreationTimeoutStr)
	return
}

func isNewlyCreated(m *machinev1alpha1.Machine) bool {
	return m.Status.CurrentStatus.Phase == machinev1alpha1.MachinePending || m.Status.CurrentStatus.Phase == machinev1alpha1.MachineAvailable
}

func hasJoined(m *machinev1alpha1.Machine) bool {
	return m.Status.CurrentStatus.Phase == machinev1alpha1.MachineRunning && strings.Contains(m.Status.LastOperation.Description, "successfully joined the cluster")
}

// GetMachineDeploymentName gets the name of the MachineDeployment associated with this Machine
func GetMachineDeploymentName(machine *machinev1alpha1.Machine) string {
	return machine.Labels["name"]
}

// AdjustTimeout adjusts the currTimeout by the growthFactor bounded to maxTimeout. Returns 0 if there was no adjustment.
func AdjustTimeout(currTimeout time.Duration, growthFactor float64, maxTimeout time.Duration) time.Duration {
	if growthFactor <= 1.0 || currTimeout >= maxTimeout || currTimeout <= 0 {
		return 0
	}
	newTimeout := time.Duration(float64(currTimeout) * growthFactor)
	if newTimeout > maxTimeout {
		newTimeout = maxTimeout
	}
	if newTimeout == currTimeout {
		return 0
	}
	return newTimeout
}
