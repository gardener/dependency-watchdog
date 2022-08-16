package weeder

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//NewWatcher(ctx, eventHandler)
//	go watcher.Watch(ctx)

// Watcher watches a kubernetes resource
type Watcher interface {
	Watch(ctx context.Context)
}

// NewWatcher creates and returns a new Watcher object
func NewWatcher(ctx context.Context, fixPodFn func(ctx context.Context, client client.Client, pod *v1.Pod) error, wr *Weeder, selector *metav1.LabelSelector) (Watcher, error) {
	w, err := newWatchObject(ctx, wr, selector)
	if err != nil {
		return nil, err
	}
	return &podWatcher{
		ctx:      ctx,
		fixPodFn: fixPodFn,
		watch:    w,
	}, nil
}

// podWatcher watches a pod
type podWatcher struct {
	ctx      context.Context
	fixPodFn func(ctx context.Context, client client.Client, pod *v1.Pod) error
	watch    watch.Interface
	wr       *Weeder
	selector *metav1.LabelSelector
}

func (pw *podWatcher) Watch(ctx context.Context) {
	defer func(w watch.Interface) {
		// to deal with case when recreateWatch returns error
		if w != nil {
			w.Stop()
		}
	}(pw.watch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-pw.watch.ResultChan():
			if !ok {
				if err := pw.recreateWatch(); err != nil {
					pw.wr.logger.Error(err, "error watching pods", "service", pw.wr.endpoints.Name, "selector", pw.selector)
					return
				}
				continue
			}

			if !canProcessEvent(event) {
				continue
			}
			targetPod := event.Object.(*v1.Pod)
			if err := pw.fixPodFn(ctx, pw.wr.Client, targetPod); err != nil {
				pw.wr.logger.Error(err, "error processing pod ", "podName", event.Object.(*v1.Pod).Name)
			}
		}
	}
}

func newWatchObject(ctx context.Context, wr *Weeder, selector *metav1.LabelSelector) (watch.Interface, error) {
	w, err := wr.SeedClient.CoreV1().Pods(wr.namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("error watching pods with selector %s", selector.String())
	}
	return w, nil
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

func (pw *podWatcher) recreateWatch() error {
	pw.watch.Stop()
	w, err := newWatchObject(pw.ctx, pw.wr, pw.selector)
	pw.watch = w
	return err
}
