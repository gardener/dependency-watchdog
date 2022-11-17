// Copyright 2022 SAP SE or an SAP affiliate company
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

package controllers

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ReadyEndpoints is a predicate to allow events for only ready endpoints. Endpoint is considered ready
// when there is at least a single endpoint subset that has at least one IP address assigned.
func ReadyEndpoints(logger logr.Logger) predicate.Predicate {
	log := logger.WithValues("predicate", "ReadyEndpointsPredicate")
	isEndpointReady := func(obj runtime.Object) bool {
		ep, ok := obj.(*v1.Endpoints)
		if !ok || ep == nil {
			return false
		}
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) > 0 {
				return true
			}
		}
		log.Info("Endpoint does not have any IP address. Skipping processing this endpoint.", "namespace", ep.Namespace, "endpoint", ep.Name)
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isEndpointReady(event.Object)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			isNewReady := isEndpointReady(event.ObjectNew)
			isOldReady := isEndpointReady(event.ObjectOld)

			if isNewReady && !isOldReady {
				return true
			}

			return false
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return isEndpointReady(event.Object)
		},
	}
}

// MatchingEndpoints is a predicate to allow events for only configured endpoints
func MatchingEndpoints(epMap map[string]wapi.DependantSelectors) predicate.Predicate {
	isMatchingEndpoints := func(obj runtime.Object, epMap map[string]wapi.DependantSelectors) bool {
		ep, ok := obj.(*v1.Endpoints)
		if !ok || ep == nil {
			return false
		}
		_, exists := epMap[ep.Name]
		return exists
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isMatchingEndpoints(event.Object, epMap)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			return isMatchingEndpoints(event.ObjectNew, epMap)
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return isMatchingEndpoints(event.Object, epMap)
		},
	}
}
