package util

import (
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"slices"
)

const workerPoolLabel = "worker.gardener.cloud/pool"

// DefaultSkipMeltdownProtectionForNodesWithConditions are the default node conditions on which meltdown protection for a node will be skipped.
// These conditions are borrowed from MCM which uses these conditions to decide if a node is unhealthy which is then replaced.
// NOTE: If these default set of node conditions are changed in MCM, make sure to change it here as well.
var DefaultSkipMeltdownProtectionForNodesWithConditions = []string{"KernelDeadlock", "ReadonlyFilesystem", "DiskPressure", "NetworkUnavailable"}

// IsAnyNodeConditionSet return true if the node has at least one of the given conditions set to true.
func IsAnyNodeConditionSet(node *corev1.Node, conditionNames []string) bool {
	for _, nc := range node.Status.Conditions {
		if slices.Contains(conditionNames, string(nc.Type)) {
			if nc.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

// GetEffectiveNodeConditionsForWorkers initializes the node conditions per worker.
func GetEffectiveNodeConditionsForWorkers(shoot *v1beta1.Shoot) map[string][]string {
	workerNodeConditions := make(map[string][]string)
	for _, worker := range shoot.Spec.Provider.Workers {
		if worker.MachineControllerManagerSettings != nil {
			workerNodeConditions[worker.Name] = GetSliceOrDefault(worker.MachineControllerManagerSettings.NodeConditions, DefaultSkipMeltdownProtectionForNodesWithConditions)
		}
	}
	return workerNodeConditions
}

// GetEffectiveNodeConditions returns the effective node conditions for the pool where this node belongs.
func GetEffectiveNodeConditions(node *corev1.Node, workerNodeConditions map[string][]string) []string {
	if poolName, foundWorkerPoolLabel := node.Labels[workerPoolLabel]; foundWorkerPoolLabel {
		if conditions, foundWorkerPoolNodeConditions := workerNodeConditions[poolName]; foundWorkerPoolNodeConditions {
			return conditions
		}
	}
	return DefaultSkipMeltdownProtectionForNodesWithConditions
}
