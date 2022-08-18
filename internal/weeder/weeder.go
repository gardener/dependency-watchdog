package weeder

import (
	"context"
	"fmt"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const crashLoopBackOff = "CrashLoopBackOff"

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

func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, seedClient kubernetes.Interface, ep *v1.Endpoints, logger logr.Logger) *Weeder {
	ctx, cancelFn := context.WithTimeout(parentCtx, *config.WatchDuration)
	dependantSelectors := config.ServicesAndDependantSelectors[ep.Name]
	return &Weeder{
		namespace:          namespace,
		endpoints:          ep,
		ctrlClient:         ctrlClient,
		watchClient:        seedClient,
		dependantSelectors: dependantSelectors,
		ctx:                ctx,
		cancelFn:           cancelFn,
		logger:             logger,
	}
}

func (w *Weeder) Run() {
	for _, ps := range w.dependantSelectors.PodSelectors {
		pw := podWatcher{
			eventHandlerFn: shootPodIfNecessary,
			selector:       ps,
			weeder:         w,
		}
		go pw.watch()
	}
	// weeder should wait till the context expires
	<-w.ctx.Done()
}

func shootPodIfNecessary(ctx context.Context, client client.Client, podNamespaceName types.NamespacedName) error {
	// Validate pod status again before shoot it out.
	logger := log.FromContext(ctx)
	var latestPod *v1.Pod
	err := client.Get(ctx, podNamespaceName, latestPod)
	if err != nil {
		return fmt.Errorf("error getting pod %s", latestPod.Name)
	}
	if !shouldDeletePod(latestPod) {
		return nil
	}
	logger.Info("Deleting pod", "name", latestPod.Name)
	return client.Delete(ctx, latestPod)
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
