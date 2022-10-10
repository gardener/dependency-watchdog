package controllers

import (
	v12 "github.com/gardener/dependency-watchdog/api/weeder/v1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ReadyEndpoints is a predicate to allow events for only ready endpoints. Endpoint is considered ready
// when there at least a single endpoint subset that has at least one IP address assigned.
func ReadyEndpoints(logger logr.Logger) predicate.Predicate {
	log := logger.WithValues("predicate", "ReadyEndpointsPredicate")
	isEndpointReady := func(obj runtime.Object) bool {
		ep, ok := obj.(*v1.Endpoints)
		if !ok || ep == nil {
			return false
		}
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) != 0 {
				return true
			}
		}
		log.Info("Endpoint does not have any endpoint subset. Skipping pod terminations.", "namespace", ep.Namespace, "endpoint", ep.Name)
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isEndpointReady(event.Object)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			isNewReady := isEndpointReady(event.ObjectNew)
			isOldReady := isEndpointReady(event.ObjectOld)

			if isNewReady && isOldReady {
				return false
			} else if isNewReady {
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
func MatchingEndpoints(epMap map[string]v12.DependantSelectors) predicate.Predicate {
	isMatchingEndpoints := func(obj runtime.Object, epMap map[string]v12.DependantSelectors) bool {
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
