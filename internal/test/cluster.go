// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"fmt"
	"math/rand"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/pointer"
)

// CreateClusterResource creates a test cluster and shoot resources.
// This should only be used for unit testing.
func CreateClusterResource(numWorkers int, nodeMonitorGracePeriod *metav1.Duration, k8sVersion string, rawShoot bool) (*gardenerv1alpha1.Cluster, *gardencorev1beta1.Shoot, error) {
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
	shoot := CreateShoot(seed.Name, numWorkers, nodeMonitorGracePeriod, k8sVersion)
	if rawShoot {
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

// CreateShoot creates a shoot resources.
// This should only be used for unit testing.
func CreateShoot(seedName string, numWorkers int, nodeMonitorGracePeriod *metav1.Duration, k8sVersion string) gardencorev1beta1.Shoot {
	end := "00 08 * * 1,2,3,4,5"
	start := "30 19 * * 1,2,3,4,5"
	location := "Asia/Calcutta"

	return gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shoot--test",
		},
		Spec: gardencorev1beta1.ShootSpec{
			Hibernation: &gardencorev1beta1.Hibernation{
				Enabled: pointer.Bool(false),
				Schedules: []gardencorev1beta1.HibernationSchedule{
					{End: &end, Start: &start, Location: &location},
				},
			},
			Kubernetes: gardencorev1beta1.Kubernetes{
				KubeControllerManager: &gardencorev1beta1.KubeControllerManagerConfig{
					NodeMonitorGracePeriod: nodeMonitorGracePeriod,
				},
				Version: k8sVersion,
			},
			Provider: gardencorev1beta1.Provider{
				Type:    "aws",
				Workers: createWorkers(numWorkers),
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

func createWorkers(numWorkers int) []gardencorev1beta1.Worker {
	workers := make([]gardencorev1beta1.Worker, 0, numWorkers)
	for i := 0; i < numWorkers; i++ {
		mx := rand.Int31n(5)
		w := gardencorev1beta1.Worker{
			Name:    fmt.Sprintf("worker-pool-%d", i),
			Machine: gardencorev1beta1.Machine{},
			Maximum: mx,
			Minimum: 1,
		}
		workers = append(workers, w)
	}
	return workers
}
