package weeder

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"time"

	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	internalutils "github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Weeder struct {
	namespace string
	endpoints *v1.Endpoints
	client.Client
	SeedClient         kubernetes.Interface
	logger             logr.Logger
	dependantSelectors wapi.DependantSelectors
	ctx                context.Context
	cancelFn           context.CancelFunc
}

func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, seedClient kubernetes.Interface, ep *v1.Endpoints, logger logr.Logger) *Weeder {
	ctx, cancelFn := context.WithTimeout(parentCtx, *config.WatchDuration)
	dependantSelectors := config.ServicesAndDependantSelectors[ep.Name]
	return &Weeder{
		namespace:          namespace,
		endpoints:          ep,
		Client:             ctrlClient,
		SeedClient:         seedClient,
		logger:             logger,
		dependantSelectors: dependantSelectors,
		ctx:                ctx,
		cancelFn:           cancelFn,
	}
}

func (w *Weeder) Run(watchDuration *time.Duration) {
	for _, ps := range w.dependantSelectors.PodSelectors {
		watcher, _ := NewWatcher(w.ctx, shootPodIfNecessary, w, ps)
		go watcher.Watch(w.ctx)
	}
	w.logger.Info("Waiting for pods in Crashloopbackoff for a period of %s", watchDuration.String())

	// weeder should wait till the context expires
	<-w.ctx.Done()
}

func shootPodIfNecessary(ctx context.Context, client client.Client, pod *v1.Pod) error {
	// Validate pod status again before shoot it out.
	logger := log.FromContext(ctx)
	latestPod := new(v1.Pod)
	err := client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, latestPod)
	if err != nil {
		return fmt.Errorf("error getting pod %s", latestPod.Name)
	}
	if !internalutils.ShouldDeletePod(latestPod) {
		return nil
	}
	logger.Info("Deleting pod", "name", latestPod.Name)
	return client.Delete(ctx, latestPod)
}
