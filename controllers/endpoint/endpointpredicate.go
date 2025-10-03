// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ReadyEndpoints is a predicate to allow events for only ready endpoints. Endpoint is considered ready
// when there is at least a single endpoint subset that has at least one IP address assigned.
func ReadyEndpoints(logger logr.Logger) predicate.Predicate {
	log := logger.WithValues("predicate", "ReadyEndpointsPredicate")
	isEndpointReady := func(obj runtime.Object) bool {
		epSlice, ok := obj.(*discoveryv1.EndpointSlice)
		if !ok || epSlice == nil {
			return false
		}
		for _, endpoint := range epSlice.Endpoints {
			// check if there is at least one endpoint with condition ready as false and return false
			if endpoint.Conditions.Ready == nil || (endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready) {
				log.Info("Not all endpoints in the endpoint slice are ready", "namespace", epSlice.Namespace, "endpoint", epSlice.Name)
				return false
			}
		}
		return true
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

		DeleteFunc: func(_ event.DeleteEvent) bool {
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
		epSlice, ok := obj.(*discoveryv1.EndpointSlice)
		if !ok || epSlice == nil {
			return false
		}

		// _, exists := epMap[epSlice.Labels[wapi.ServiceNameLabel]]
		_, exists := epMap[epSlice.Name]
		return exists
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isMatchingEndpoints(event.Object, epMap)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			return isMatchingEndpoints(event.ObjectNew, epMap)
		},

		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return isMatchingEndpoints(event.Object, epMap)
		},
	}
}
