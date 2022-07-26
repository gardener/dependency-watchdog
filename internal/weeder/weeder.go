package weeder

import (
	"context"
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Weeder struct {
	namespace string
	endpoints *v1.Endpoints
	client.Client
	logger   log.Logger
	config   *wapi.Config
	ctx      context.Context
	cancelFn context.CancelFunc
}

//TODO create a construction function for Weeder

func (w *Weeder) Close() {
	//TODO
}

func (w *Weeder) isClosed() {
	//TODO
}

func (w *Weeder) Run() {
	// Invoke go watchAndWeed for each LabelSelector configured for the service
}

func (w *Weeder) watchAndWeed(ctx context.Context, selector *metav1.LabelSelector) {

}
