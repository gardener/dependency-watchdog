// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package restarter

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/dependency-watchdog/pkg/restarter/api"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"

	"github.com/gardener/dependency-watchdog/pkg/multicontext"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// NewController initializes a new K8s dependency-watchdog controller with restarter.
func NewController(clientset kubernetes.Interface,
	sharedInformerFactory informers.SharedInformerFactory,
	serviceDependants *api.ServiceDependants,
	watchDuration time.Duration,
	stopCh <-chan struct{}) *Controller {
	c := &Controller{
		clientset:         clientset,
		informerFactory:   sharedInformerFactory,
		endpointInformer:  sharedInformerFactory.Core().V1().Endpoints().Informer(),
		endpointLister:    sharedInformerFactory.Core().V1().Endpoints().Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Endpoints"),
		stopCh:            stopCh,
		serviceDependants: serviceDependants,
		watchDuration:     watchDuration,
		Multicontext:      multicontext.New(),
		LeaderElection: componentbaseconfigv1alpha1.LeaderElectionConfiguration{
			ResourceLock: resourcelock.LeasesResourceLock,
		},
	}
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&c.LeaderElection)
	c.endpointInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueEndpoint,
		UpdateFunc: func(old, new interface{}) {
			newEp := new.(*v1.Endpoints)
			oldEp := old.(*v1.Endpoints)
			if newEp.ResourceVersion == oldEp.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			c.enqueueEndpoint(new)
		},
	})
	c.hasSynced = c.endpointInformer.HasSynced
	return c
}

// enqueueEndpoint takes an Endpoint resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Endpoints.
func (c *Controller) enqueueEndpoint(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Errorf("Error parsing key %s: %s", key, err)
		return
	}

	// Skip resources from other namespaces if namespace is specified explicitly in the configuration.
	if c.serviceDependants.Namespace != "" && c.serviceDependants.Namespace != namespace {
		return
	}

	// Skip if the resource is not found in the services configured as to be watched.
	if _, ok := c.serviceDependants.Services[name]; !ok {
		return
	}

	c.workqueue.AddRateLimited(key)
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting restarter controller")

	klog.Info("Starting informer factory.")
	c.informerFactory.Start(c.stopCh)

	go c.Multicontext.Start(c.stopCh)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(c.stopCh, c.hasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch workers to process VPA resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, c.stopCh)
	}

	klog.Info("Started workers")
	<-c.stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the processEndpoint.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {

			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		if err := c.processEndpoint(context.TODO(), key); err != nil {

			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %v, requeuing", key, err)
		}

		c.workqueue.Forget(obj)

		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) processEndpoint(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	if c.serviceDependants.Namespace != "" && namespace != c.serviceDependants.Namespace {
		return nil
	}

	ep, err := c.clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// The endpoint resource may no longer exist, in which case we stop
		// processing.
		if apierrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("endpoint '%s' in work queue no longer exists", key))
			// Cancel any existing context to pro-actively avoid shooting pods accidentally.
			c.ContextCh <- &multicontext.ContextMessage{
				Key:      key,
				CancelFn: nil,
			}
			return nil
		}
		return err
	}
	srv, ok := c.serviceDependants.Services[name]
	if !ok {
		return nil
	}
	klog.Infof("Processing endpoint: %s", key)
	if !IsReadyEndpointPresentInSubsets(ep.Subsets) {
		klog.Infof("Endpoint %s does not have any endpoint subset. Skipping pod terminations.", ep.Name)
		// Cancel any existing context to pro-actively avoid shooting pods accidentally.
		c.ContextCh <- &multicontext.ContextMessage{
			Key:      key,
			CancelFn: nil,
		}
		return nil
	}

	go func() {
		klog.Infof("Watching for pods in CrashLoopBackOff for a period of %s", c.watchDuration.String())
		ctx, cancelFn := context.WithTimeout(ctx, c.watchDuration)
		defer cancelFn()

		c.ContextCh <- &multicontext.ContextMessage{
			Key:      key,
			CancelFn: cancelFn,
		}

		c.shootPodsIfNecessary(ctx, namespace, srv)
		select {
		case <-ctx.Done():
			c.ContextCh <- &multicontext.ContextMessage{
				Key:      key,
				CancelFn: nil,
			}
			return
		case <-c.stopCh:
			return
		}
	}()
	return nil
}

func (c *Controller) shootPodsIfNecessary(ctx context.Context, namespace string, srv api.Service) error {
	for _, dependantPod := range srv.Dependants {
		go func(depPods api.DependantPods) {
			err := c.shootDependentPodsIfNecessary(ctx, namespace, &depPods)
			if err != nil {
				klog.Errorf("Error processing dependents pods: %s", err)
			}
		}(dependantPod)
	}
	return nil
}

func (c *Controller) shootDependentPodsIfNecessary(ctx context.Context, namespace string, depPods *api.DependantPods) error {
	selector, err := metav1.LabelSelectorAsSelector(depPods.Selector)
	if err != nil {
		return fmt.Errorf("error converting label selector to selector %s", depPods.Selector.String())
	}

	for {
		retry, err := func() (bool, error) {
			w, err := c.clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
				LabelSelector: selector.String(),
			})
			if err != nil {
				return false, fmt.Errorf("error watching pods with selector %s", selector.String())
			}

			defer w.Stop()

			for {
				select {
				case <-ctx.Done():
					klog.Infof("Watch duration completed for pods with selector: %s", selector.String())
					return false, nil
				case <-c.stopCh:
					klog.Infof("Received stop signal. Stopping watch for pods with selector: %s", selector.String())
					return false, nil
				case ev, ok := <-w.ResultChan():
					if !ok {
						klog.Infof("Received error from watch channel. Will restart the watch with selector: %s", selector.String())
						return true, nil
					}
					if ev.Type != watch.Added && ev.Type != watch.Modified {
						klog.Infof("Skipping event type: %s", ev.Type)
						continue
					}
					switch pod := ev.Object.(type) {
					case *v1.Pod:
						err := c.processPod(ctx, pod)
						if err != nil {
							klog.Errorf("error processing pod %s: %v", pod.Name, err.Error())
						}
					default:
						klog.Errorf("Unknown type received from watch channel: %T", ev.Object)
					}
				}
			}
		}()
		if !retry {
			return err
		}
	}
}

func (c *Controller) processPod(ctx context.Context, pod *v1.Pod) error {
	// Validate pod status again before shoot it out.
	po, err := c.clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting pod %s", pod.Name)
	}
	if !ShouldDeletePod(po) {
		return nil
	}
	klog.Infof("Deleting pod: %v", po.Name)
	return c.clientset.CoreV1().Pods(po.Namespace).Delete(ctx, po.Name, metav1.DeleteOptions{})
}
