// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package weeder

import (
	"context"

	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const crashLoopBackOff = "CrashLoopBackOff"

// Weeder represents an actor which will be responsible for watching dependent pods and weeding them out if they
// are in CrashLoopBackOff.
type Weeder struct {
	namespace          string
	endpointSlice      *discoveryv1.EndpointSlice
	ctrlClient         client.Client
	watchClient        kubernetes.Interface
	dependantSelectors wapi.DependantSelectors
	ctx                context.Context
	cancelFn           context.CancelFunc
	logger             logr.Logger
}

// NewWeeder creates a new Weeder for a service/endpoint.
func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, seedClient kubernetes.Interface, ep *discoveryv1.EndpointSlice, logger logr.Logger) *Weeder {
	wLogger := logger.WithValues("weederRunning", true, "watchDuration", (*config.WatchDuration).String())
	ctx, cancelFn := context.WithTimeout(parentCtx, config.WatchDuration.Duration)
	dependantSelectors := config.ServicesAndDependantSelectors[ep.Labels[discoveryv1.LabelServiceName]]
	return &Weeder{
		namespace:          namespace,
		endpointSlice:      ep,
		ctrlClient:         ctrlClient,
		watchClient:        seedClient,
		dependantSelectors: dependantSelectors,
		ctx:                ctx,
		cancelFn:           cancelFn,
		logger:             wLogger,
	}
}

// Run runs the Weeder which will intern create one go-routine for dependents identified by respective PodSelector.
func (w *Weeder) Run() {
	for _, ps := range w.dependantSelectors.PodSelectors {
		go newPodWatcher(w, ps, shootPodIfNecessary).watch()
	}
	// weeder should wait till the context expires
	<-w.ctx.Done()
}

func shootPodIfNecessary(ctx context.Context, log logr.Logger, crClient client.Client, targetPod *v1.Pod) error {
	if !shouldDeletePod(targetPod) {
		return nil
	}
	log.Info("Deleting pod", "namespace", targetPod.Namespace, "podName", targetPod.Name)
	return crClient.Delete(ctx, targetPod)
}

// shouldDeletePod checks if a pod should be deleted for quicker recovery. A pod can be deleted
// only if it is not marked for deletion and is currently in CrashLoopBackOff state
func shouldDeletePod(pod *v1.Pod) bool {
	podNotMarkedForDeletion := pod.DeletionTimestamp == nil
	return podNotMarkedForDeletion && isPodInCrashloopBackoff(pod.Status)
}

// isPodInCrashloopBackoff checks if any container in a pod is in CrashLoopBackOff
func isPodInCrashloopBackoff(status v1.PodStatus) bool {
	for _, containerStatus := range status.ContainerStatuses {
		if isContainerInCrashLoopBackOff(containerStatus.State) {
			return true
		}
	}
	return false
}

// isContainerInCrashLoopBackOff checks if a container is in CrashLoopBackOff
func isContainerInCrashLoopBackOff(containerState v1.ContainerState) bool {
	return containerState.Waiting != nil && containerState.Waiting.Reason == crashLoopBackOff
}
