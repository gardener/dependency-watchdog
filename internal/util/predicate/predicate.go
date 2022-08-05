package predicate

import (
	"github.com/gardener/dependency-watchdog/api/weeder"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ReadyEndpoints is a predicate to allow events for only ready endpoints
func ReadyEndpoints() predicate.Predicate {
	isEndpointReady := func(obj runtime.Object) bool {
		ep, ok := obj.(*v1.Endpoints)
		if !ok {
			return false
		}
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) != 0 {
				return true
			}
		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isEndpointReady(event.Object)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			return isEndpointReady(event.ObjectNew)
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// RelevantEndpoints is a predicate to allow events for only configured endpoints
func RelevantEndpoints(epMap map[string]weeder.DependantSelectors) predicate.Predicate {
	isRelevantEndpoints := func(obj runtime.Object, epMap map[string]weeder.DependantSelectors) bool {
		ep, ok := obj.(*v1.Endpoints)
		if !ok {
			return false
		}
		if _, ok := epMap[ep.Name]; ok {
			return true
		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isRelevantEndpoints(event.Object, epMap)
		},

		UpdateFunc: func(event event.UpdateEvent) bool {
			return isRelevantEndpoints(event.ObjectNew, epMap)
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}
