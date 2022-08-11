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
	logger   logr.Logger
	config   *wapi.Config
	ctx      context.Context
	cancelFn context.CancelFunc
}

func NewWeeder(parentCtx context.Context, namespace string, config *wapi.Config, ctrlClient client.Client, ep *v1.Endpoints, logger logr.Logger) *Weeder {
	ctx, cancel := context.WithCancel(parentCtx)
	return &Weeder{
		namespace: namespace,
		endpoints: ep,
		Client:    ctrlClient,
		logger:    logger,
		config:    config,
		ctx:       ctx,
		cancelFn:  cancel,
	}
}

func (w *Weeder) Close() {
	w.cancelFn()
}

func (w *Weeder) isClosed() bool {
	select {
	case <-w.ctx.Done():
		return true
	default:
		return false
	}
}

func (w *Weeder) Run() {
	for _, ds := range w.config.ServicesAndDependantSelectors {
		for _, selector := range ds.PodSelectors {
			go w.watchAndWeed(w.ctx, selector)
		}
	}
}

func (w *Weeder) watchAndWeed(ctx context.Context, selector *metav1.LabelSelector) {
}
