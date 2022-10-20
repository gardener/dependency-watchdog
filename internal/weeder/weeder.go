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

package weeder

import (
	"context"

	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const crashLoopBackOff = "CrashLoopBackOff"

// Weeder represents an actor which will be responsible for watching dependent pods and weeding them out if they
// are in CrashLoopBackOff.
type Weeder struct {
	namespace          string
	endpoints          *v1.Endpoints
	ctrlClient         client.Client
	watchClient        kubernetes.Interface
	dependantSelectors wapi.DependantSelectors
	ctx                context.Context
	cancelFn           context.CancelFunc
	logger             logr.Logger
}

// NewWeeder creates a new Weeder for a service/endpoint.
func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, seedClient kubernetes.Interface, ep *v1.Endpoints, logger logr.Logger) *Weeder {
	wLogger := logger.WithValues("weederRunning", true, "watchDuration", (*config.WatchDuration).String())
	ctx, cancelFn := context.WithTimeout(parentCtx, config.WatchDuration.Duration)
	dependantSelectors := config.ServicesAndDependantSelectors[ep.Name]
	return &Weeder{
		namespace:          namespace,
		endpoints:          ep,
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
