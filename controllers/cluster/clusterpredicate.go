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

package cluster

import (
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// workerLessShoot creates predicate functions to react to length of workers in a shoot for the given cluster object.
// For shoots which do not have any workers, no probe should be registered.
// CreateEvents: For a new shoot creation, only if the shoot has workers should this predicate return true.
// UpdateEvents: If either of the old or the new shoot has workers it should return true. Required to support any future enhancements which allow a transition to/from a worker-less shoot.
// DeleteEvents: Only if there is a delete event for a shoot with workers should this predicate return true.
// GenericEvents: For any other event to be on the safe side we always return true.
func workerLessShoot(logger logr.Logger) predicate.Predicate {
	log := logger.WithValues("predicate", "workerLessShoot")
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return shootHasWorkers(event.Object, log)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return shootHasWorkers(event.Object, log)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldShootHasWorkers := shootHasWorkers(updateEvent.ObjectOld, log)
			newShootHasWorkers := shootHasWorkers(updateEvent.ObjectNew, log)
			return oldShootHasWorkers || newShootHasWorkers
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return true
		},
	}
}

// shootHasWorkers extracts the shoot from the cluster and checks if shoot has workers.
func shootHasWorkers(obj runtime.Object, logger logr.Logger) bool {
	cluster, ok := obj.(*extensionsv1alpha1.Cluster)
	if !ok {
		return false
	}
	shoot, err := extensionscontroller.ShootFromCluster(cluster)
	if err != nil {
		logger.Error(err, "Failed to extract shoot from cluster event", "cluster", cluster.Name)
		return false
	}
	return len(shoot.Spec.Provider.Workers) > 0
}
