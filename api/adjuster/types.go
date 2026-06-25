// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// AnnotationKeyEffectiveCreationTimeout is the annotation key set on the MachineDeployment that indicates
	// the effective creation timeout for all Machine's belonging to this MachineDeployment. If specified, the value for this
	// annotation takes precedence over the MachineDeployment.Spec.Template.Spec.MachineCreationTimeout.
	AnnotationKeyEffectiveCreationTimeout = "node.machine.sapcloud.io/effective-creation-timeout"
	// AnnotationKeyLastAdjustedEffectiveCreationTimeout is the annotation key set on the MachineDeployment that indicates
	// the time that the effective creation timeout was last adjusted for this MachineDeployment.
	AnnotationKeyLastAdjustedEffectiveCreationTimeout = "node.machine.sapcloud.io/last-adjusted-effective-creation-timeout"
	// AnnotationKeyMachineJoinDuration is the annotation key set on the Machine that indicates the amount of time Machine
	// took to join the cluster. The value is a Go Duration string.
	AnnotationKeyMachineJoinDuration = "node.machine.sapcloud.io/machine-join-duration"
	//DefaultCreationTimeoutMax is the max limit beyond upto which the machine-creation-timeout can be adjusted
	DefaultCreationTimeoutMax = 90 * time.Minute
	// DefaultFailureThreshold is the default value of the threshold for number of Failed Machines beyond which
	// the adjuster will revise the machine-creation-timeout.
	DefaultFailureThreshold = 2
	// StandardCreationTimeout is the standard machine creation timeout in gardener clusters if not overridden.
	// See https://github.com/gardener/gardener/blob/e140ccc402b8732499cb804190fc4fe1ce82c078/example/90-shoot.yaml#L142
	StandardCreationTimeout = 20 * time.Minute
)

// Config provides typed access to adjuster configuration. Corresponds to the config map used to configure the adjuster controller
type Config struct {
	// CreationTimeoutGrowthFactor is the growth factor used by adjuster for increasing the effective machine-creation-timeout
	CreationTimeoutGrowthFactor *float64 `json:"creationTimeoutGrowthFactor"`
	// CreationTimeoutMax is the maximum effective machine-creation-timeout set by the adjuster on MachineDeployment objects.
	CreationTimeoutMax *metav1.Duration `json:"creationTimeoutMax"`
	// FailureThreshold is the threshold for number of Failed Machines for a given instance type+zone combo following which
	// the adjuster will revise the effective machine-creation-timeout upwards by the [CreationTimeoutGrowthFactor].
	FailureThreshold *int32 `json:"failureThreshold"`
}

// ProvisionKey identifies a group of machines with the same instance
// type and availability zone for which provisioning statistics are tracked.
//
// Machines with the same instance type and availability zone share the same
// provisioning characteristics, such as capacity availability, machine join
// latency, and provisioning failure rates.
type ProvisionKey struct {
	InstanceType string
	Zone         string
}

// Controller is the facade exposed by the adjuster controller.
type Controller interface {
	reconcile.Reconciler
	// SetupWithManager sets up the controller with the controller-runtime Manager.
	SetupWithManager(manager.Manager) error
}
