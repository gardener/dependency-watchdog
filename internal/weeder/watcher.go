package weeder

import (
	"context"
	"fmt"
	"github.com/gardener/dependency-watchdog/internal/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const watchCreationRetryInterval = 500 * time.Millisecond

type podEventHandler func(ctx context.Context, client client.Client, podNamespaceName types.NamespacedName) error

// podWatcher watches a pod
type podWatcher struct {
	selector       *metav1.LabelSelector
	eventHandlerFn podEventHandler
	k8sWatch       watch.Interface
	weeder         *Weeder
}

func (pw *podWatcher) createK8sWatch(ctx context.Context) {
	operation := fmt.Sprintf("Creating kubernetes watch for namespace %s, service %s with selector %s", pw.weeder.namespace, pw.weeder.endpoints.Name, pw.selector)
	util.RetryOnError(ctx, operation, func() error {
		w, err := doCreateK8sWatch(ctx, pw.weeder.watchClient, pw.weeder.namespace, pw.selector)
		if err != nil {
			return err
		}
		pw.k8sWatch = w
		return nil
	}, watchCreationRetryInterval)
}

func (pw *podWatcher) Close() {
	if pw.k8sWatch != nil {
		pw.k8sWatch.Stop()
	}
}

func doCreateK8sWatch(ctx context.Context, client kubernetes.Interface, namespace string, selector *metav1.LabelSelector) (watch.Interface, error) {
	w, err := client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (pw *podWatcher) watch() {
	pw.createK8sWatch(pw.weeder.ctx)
	for {
		select {
		case <-pw.weeder.ctx.Done():
			pw.weeder.logger.V(5).Info("Exiting watch as context has timed-out or has been cancelled", "namespace", pw.weeder.namespace, "selector", pw.selector.String())
			return
		case event, ok := <-pw.k8sWatch.ResultChan():
			if !ok {
				pw.weeder.logger.V(4).Info("Watch has stopped, recreating kubernetes watch", "namespace", pw.weeder.namespace, "service", pw.weeder.endpoints.Name, "selector", pw.selector, pw.selector.String())
				pw.createK8sWatch(pw.weeder.ctx)
				continue
			}
			if !canProcessEvent(event) {
				continue
			}
			targetPod := event.Object.(*v1.Pod)
			if err := pw.eventHandlerFn(pw.weeder.ctx, pw.weeder.ctrlClient, types.NamespacedName{Namespace: targetPod.Namespace, Name: targetPod.Name}); err != nil {
				pw.weeder.logger.Error(err, "error processing pod ", "podName", event.Object.(*v1.Pod).Name)
			}
		}
	}
}

func canProcessEvent(ev watch.Event) bool {
	if ev.Type == watch.Added || ev.Type == watch.Modified {
		switch ev.Object.(type) {
		case *v1.Pod:
			return true
		default:
			return false
		}
	}
	return false
}
