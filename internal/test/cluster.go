// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
)

// ClusterBuilder is a builder for the cluster resource.
type ClusterBuilder struct {
	numWorkers             int
	nodeMonitorGracePeriod *metav1.Duration
	rawShoot               bool
	workerNodeConditions   [][]string
}

// NewClusterBuilder creates a new instance of ClusterBuilder.
func NewClusterBuilder() *ClusterBuilder {
	return &ClusterBuilder{}
}

// WithWorkerCount sets the worker count for the cluster.
func (b *ClusterBuilder) WithWorkerCount(workerCount int) *ClusterBuilder {
	b.numWorkers = workerCount
	return b
}

// WithNodeMonitorGracePeriod sets the node monitor grace period for the cluster.
func (b *ClusterBuilder) WithNodeMonitorGracePeriod(nodeMonitorGracePeriod *metav1.Duration) *ClusterBuilder {
	b.nodeMonitorGracePeriod = nodeMonitorGracePeriod
	return b
}

// WithRawShoot sets the raw shoot for the cluster.
func (b *ClusterBuilder) WithRawShoot(rawShoot bool) *ClusterBuilder {
	b.rawShoot = rawShoot
	return b
}

// WithWorkerNodeConditions sets the worker node conditions for the cluster.
func (b *ClusterBuilder) WithWorkerNodeConditions(workerNodeConditions [][]string) *ClusterBuilder {
	b.workerNodeConditions = workerNodeConditions
	return b
}

// Build builds the cluster resource.
func (b *ClusterBuilder) Build() (*gardenerv1alpha1.Cluster, *gardencorev1beta1.Shoot, error) {
	cloudProfile := gardencorev1beta1.CloudProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws",
		},
	}
	seed := gardencorev1beta1.Seed{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-aws",
		},
	}
	cluster := gardenerv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shoot--test",
		},
		Spec: gardenerv1alpha1.ClusterSpec{
			CloudProfile: runtime.RawExtension{
				Object: &cloudProfile,
			},
			Seed: runtime.RawExtension{
				Object: &seed,
			},
		},
	}
	shoot := createShoot(seed.Name, b.numWorkers, b.workerNodeConditions, b.nodeMonitorGracePeriod)
	if b.rawShoot {
		shootBytes, err := json.Marshal(shoot)
		if err != nil {
			return nil, nil, err
		}
		cluster.Spec.Shoot.Raw = shootBytes
	} else {
		cluster.Spec.Shoot.Object = &shoot
	}
	return &cluster, &shoot, nil
}

// createShoot creates a shoot resources.
// This should only be used for unit testing.
func createShoot(seedName string, workerCount int, workerNodeConditions [][]string, nodeMonitorGracePeriod *metav1.Duration) gardencorev1beta1.Shoot {
	end := "00 08 * * 1,2,3,4,5"
	start := "30 19 * * 1,2,3,4,5"
	location := "Asia/Calcutta"

	return gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shoot--test",
		},
		Spec: gardencorev1beta1.ShootSpec{
			Hibernation: &gardencorev1beta1.Hibernation{
				Enabled: ptr.To(false),
				Schedules: []gardencorev1beta1.HibernationSchedule{
					{End: &end, Start: &start, Location: &location},
				},
			},
			Kubernetes: gardencorev1beta1.Kubernetes{
				KubeControllerManager: &gardencorev1beta1.KubeControllerManagerConfig{
					NodeMonitorGracePeriod: nodeMonitorGracePeriod,
				},
			},
			Provider: gardencorev1beta1.Provider{
				Type:    "aws",
				Workers: createWorkers(workerCount, workerNodeConditions),
			},
		},
		Status: gardencorev1beta1.ShootStatus{
			IsHibernated: false,
			SeedName:     &seedName,
			LastOperation: &gardencorev1beta1.LastOperation{
				Type:  gardencorev1beta1.LastOperationTypeCreate,
				State: gardencorev1beta1.LastOperationStateSucceeded,
			},
		},
	}
}

// createWorkers creates worker resources with the given count and corresponding node conditions.
func createWorkers(workerCount int, workerNodeConditions [][]string) []gardencorev1beta1.Worker {
	workers := make([]gardencorev1beta1.Worker, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		mx := rand.Intn(5)
		w := gardencorev1beta1.Worker{
			Name:    rand.String(4),
			Machine: gardencorev1beta1.Machine{},
			Maximum: int32(mx), // #nosec G115 -- lies between 0 and 5.
			Minimum: 1,
		}
		if i < len(workerNodeConditions) {
			w.MachineControllerManagerSettings = &gardencorev1beta1.MachineControllerManagerSettings{
				NodeConditions: workerNodeConditions[i],
			}
		}
		workers = append(workers, w)
	}
	return workers
}
