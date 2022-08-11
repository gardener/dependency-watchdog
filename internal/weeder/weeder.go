package weeder

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Weeder struct {
	namespace string
	endpoints *v1.Endpoints
	client.Client
	logger             logr.Logger
	dependantSelectors wapi.DependantSelectors
	ctx                context.Context
	cancelFn           context.CancelFunc
}

func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, ep *v1.Endpoints, logger logr.Logger) *Weeder {
	ctx, cancelFn := context.WithTimeout(parentCtx, *config.WatchDuration)
	dependantSelectors := config.ServicesAndDependantSelectors[ep.Name]
	return &Weeder{
		namespace:          namespace,
		endpoints:          ep,
		Client:             ctrlClient,
		logger:             logger,
		dependantSelectors: dependantSelectors,
		ctx:                ctx,
		cancelFn:           cancelFn,
	}
}

func (w *Weeder) Run() {
	for _, ps := range w.dependantSelectors.PodSelectors {
		go w.watchAndWeed(w.ctx, ps)
	}
}

/*
	NewWatcher(ctx, eventHandler)
	go watcher.Watch(ctx)

	type Watcher interface {
		Watch(ctx)
	}

	func NewWatcher(ctx, func(ctx) error) (Watcher, error){
		creates selector
		creates an initial watch using the selector
		watch is then set in the podWatcher
	}

	type podWatcher struct {
		ctx
		eventHandlerFn func(ctx) error
		watch
	}

	func (pw *podWatcher) Watch(ctx) {
		defer pw.watch.Stop()
		for {
			select {
				case <-ctx.Done:
					return
				case event, ok <- w.ResultChan:
					if !ok {
						pw.recreateWatch()
						continue
					}
					if !canProcess(event) {
						continue
					}
					w.eventHandlerFn()
			}
		}
	}
	func canProcess(watch.Event) bool {
	}

	func (pw *podWatcher) recreateWatch() {
		pw.watcher.Stop()

	}
*/

func (w *Weeder) watchAndWeed(ctx context.Context, podSelector *metav1.LabelSelector) {
	_, err := metav1.LabelSelectorAsSelector(podSelector)
	if err != nil {
		w.logger.Error(err, "This is unexpected as all selectors have already been vetted. Cancelling this weeder", "namespace", w.namespace, "endpoint", w.endpoints, "podSelector", podSelector)
		w.cancelFn()
		return
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.V(4).Info("Watch duration completed for weeder, exiting watch", "namespace", w.namespace, "endpoint", w.endpoints)
			return
		default:

		}
	}

}
