// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"slices"

	"github.com/go-logr/logr"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ReadyEndpoints is a predicate to allow events for only ready endpoints. An EndpointSlice
// is considered ready when at least one of its endpoints has Conditions.Ready set to true.
func ReadyEndpoints(logger logr.Logger) predicate.Predicate {
	log := logger.WithValues("predicate", "ReadyEndpointsPredicate")
	isEndpointReady := func(obj runtime.Object) bool {
		epSlice, ok := obj.(*discoveryv1.EndpointSlice)
		if !ok || epSlice == nil {
			return false
		}
		for _, endpoint := range epSlice.Endpoints {
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				return true
			}
		}
		log.Info("Endpoint slice has no ready endpoints", "namespace", epSlice.Namespace, "name", epSlice.Name)
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

		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return isEndpointReady(event.Object)
		},
	}
}

// MatchingEndpoints is a predicate to allow events for only configured endpoints
func MatchingEndpoints(svcNames []string) predicate.Predicate {
	isMatchingEndpoints := func(obj runtime.Object) bool {
		epSlice, ok := obj.(*discoveryv1.EndpointSlice)
		if !ok || epSlice == nil {
			return false
		}
		epSvcName, ok := epSlice.Labels[discoveryv1.LabelServiceName] // make sure the label exists
		if !ok {
			return false
		}
		return slices.Contains(svcNames, epSvcName)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isMatchingEndpoints(event.Object)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			return isMatchingEndpoints(event.ObjectNew)
		},

		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return isMatchingEndpoints(event.Object)
		},
	}
}
