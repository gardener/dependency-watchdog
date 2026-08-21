// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AnnotationKeyEffectiveCreationTimeout is the annotation key set on the MachineDeployment that indicates
	// the effective creation timeout for all Machine's belonging to this MachineDeployment. If specified, the value for this
	// annotation takes precedence over the MachineDeployment.Spec.Template.Spec.MachineCreationTimeout.
	AnnotationKeyEffectiveCreationTimeout = "node.machine.sapcloud.io/effective-creation-timeout"
	// AnnotationKeyEffectiveCreationTimeoutLastAdjustedAt is the annotation key set on the MachineDeployment that indicates
	// the time that the effective creation timeout was last adjusted for this MachineDeployment.
	AnnotationKeyEffectiveCreationTimeoutLastAdjustedAt = "node.machine.sapcloud.io/effective-creation-timeout-last-adjusted-at"
	//DefaultCreationTimeoutGrowthFactor is the default value for growth of the effective-creation-timeout.
	DefaultCreationTimeoutGrowthFactor = 2.0
	//DefaultCreationTimeoutMax is the max limit beyond upto which the machine-creation-timeout can be adjusted
	DefaultCreationTimeoutMax = 90 * time.Minute
	// DefaultMachineFailureFractionThreshold is the default value of the threshold fraction for Failed Machines corresponding
	// to a [adjustapi.ProvisionKey]. Once breached, the adjuster will revise the effective machine-creation-timeout upwards
	// by the [CreationTimeoutGrowthFactor].
	DefaultMachineFailureFractionThreshold = 0.2
	// StandardCreationTimeout is the standard machine creation timeout in gardener clusters if not overridden.
	// See https://github.com/gardener/gardener/blob/e140ccc402b8732499cb804190fc4fe1ce82c078/example/90-shoot.yaml#L142
	StandardCreationTimeout = 20 * time.Minute
)

// Config provides typed access to adjuster configuration. Corresponds to the config map used to configure the adjuster controller
type Config struct {
	// CreationTimeoutGrowthFactor is the growth factor used by adjuster for increasing the effective machine-creation-timeout
	// If unspecified, the default value is [DefaultCreationTimeoutGrowthFactor].
	CreationTimeoutGrowthFactor *float64 `json:"creationTimeoutGrowthFactor"`
	// CreationTimeoutMax is the maximum effective machine-creation-timeout set by the adjuster on MachineDeployment objects.
	// If unspecified, the default value is [DefaultCreationTimeoutMax]
	CreationTimeoutMax *metav1.Duration `json:"creationTimeoutMax"`
	// MachineFailureFractionThreshold is the fraction  for Failed Machines corresponding to a [adjustapi.ProvisionKey].
	// Once breached, the adjuster will revise the effective machine-creation-timeout upwards by the
	// [CreationTimeoutGrowthFactor].
	// If unspecified, the default value is [DefaultFailureThreshold]
	MachineFailureFractionThreshold *float64 `json:"machineFailureFractionThreshold"`
}

// MachineProvisionKey identifies a group of machines with the same instance
// type and availability zone for which provisioning statistics are tracked.
//
// Machines with the same instance type and availability zone share the same
// provisioning characteristics, such as capacity availability, machine join
// latency, and provisioning failure rates.
type MachineProvisionKey struct {
	InstanceType string
	Zone         string
}

func (k MachineProvisionKey) String() string {
	return fmt.Sprintf("(instanceType=%s,Zone=%s)", k.InstanceType, k.Zone)
}
