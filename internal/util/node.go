// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"slices"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machineutils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const nodeNameLabel = "node"

// DefaultUnhealthyNodeConditions are the default node conditions which indicate that the node is unhealthy.
// These conditions are borrowed from MCM where these conditions are used to decide if a node is unhealthy and should be replaced.
// NOTE: If these default set of node conditions are changed in MCM, make sure to change it here as well.
var DefaultUnhealthyNodeConditions = []string{"KernelDeadlock", "ReadonlyFilesystem", "DiskPressure", "NetworkUnavailable"}

// IsNodeHealthyByConditions determines if a node is healthy by checking that none of the given unhealthyWorkerConditionNames have Status set to true.
func IsNodeHealthyByConditions(logger logr.Logger, node *corev1.Node, unhealthyWorkerConditionNames []string) bool {
	for _, nc := range node.Status.Conditions {
		if slices.Contains(unhealthyWorkerConditionNames, string(nc.Type)) {
			if nc.Status == corev1.ConditionTrue {
				logger.V(4).Info("Node is unhealthy due to condition", "nodeName", node.Name, "condition", nc.Type)
				return false
			}
		}
	}
	return true
}

// IsNodeManagedByMCM determines if a node is managed by MCM by checking if the node has the annotation nodeNotManagedByMCMAnnotationKey"node.machine.sapcloud.io/not-managed-by-mcm" set.
func IsNodeManagedByMCM(node *corev1.Node) bool {
	return !metav1.HasAnnotation(node.ObjectMeta, machineutils.NotManagedByMCM)
}

// GetEffectiveNodeConditionsForWorkers initializes the node conditions per worker.
func GetEffectiveNodeConditionsForWorkers(shoot *v1beta1.Shoot) map[string][]string {
	workerNodeConditions := make(map[string][]string)
	for _, worker := range shoot.Spec.Provider.Workers {
		if worker.MachineControllerManagerSettings != nil {
			workerNodeConditions[worker.Name] = GetSliceOrDefault(worker.MachineControllerManagerSettings.NodeConditions, DefaultUnhealthyNodeConditions)
		}
	}
	return workerNodeConditions
}

// GetWorkerUnhealthyNodeConditions returns the configured node conditions for the pool where this node belongs.
// Worker name is extracted from the node labels.
func GetWorkerUnhealthyNodeConditions(node *corev1.Node, workerNodeConditions map[string][]string) []string {
	if poolName, foundWorkerPoolLabel := node.Labels[gardencorev1beta1constants.LabelWorkerPool]; foundWorkerPoolLabel {
		if conditions, foundWorkerPoolNodeConditions := workerNodeConditions[poolName]; foundWorkerPoolNodeConditions {
			return conditions
		}
	}
	return DefaultUnhealthyNodeConditions
}

// GetMachineNotInFailedOrTerminatingState returns the machine corresponding to the node which is not in failed or terminating phase.
// It will return nil if no machine is found corresponding to the node or if the machine is in failed or terminating phase.
func GetMachineNotInFailedOrTerminatingState(nodeName string, machines []v1alpha1.Machine) *v1alpha1.Machine {
	for _, machine := range machines {
		nodeLabelValue := machine.Labels[nodeNameLabel]
		if nodeLabelValue == nodeName && machine.Status.CurrentStatus.Phase != v1alpha1.MachineFailed && machine.Status.CurrentStatus.Phase != v1alpha1.MachineTerminating {
			return &machine
		}
	}
	return nil
}

// IsNodeUndergoingInPlaceUpdate checks if the node is undergoing an in-place update by checking the InPlaceUpdate condition.
func IsNodeUndergoingInPlaceUpdate(node *corev1.Node) bool {
	return slices.ContainsFunc(node.Status.Conditions, func(condition corev1.NodeCondition) bool {
		return condition.Type == v1alpha1.NodeInPlaceUpdate && sets.New(v1alpha1.ReadyForUpdate, v1alpha1.UpdateFailed).Has(condition.Reason)
	})
}
