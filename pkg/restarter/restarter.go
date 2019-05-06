// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package restarter

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	watch "k8s.io/apimachinery/pkg/watch"

	"k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

func NewController(clientset *kubernetes.Clientset,
	sharedInformerFactory informers.SharedInformerFactory,
	serviceDependants *ServiceDependants,
	watchDuration time.Duration,
	stopCh <-chan struct{}) *Controller {
	c := &Controller{
		clientset:         clientset,
		endpointInformer:  sharedInformerFactory.Core().V1().Endpoints().Informer(),
		endpointLister:    sharedInformerFactory.Core().V1().Endpoints().Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Endpoints"),
		stopCh:            stopCh,
		serviceDependants: serviceDependants,
		watchDuration:     watchDuration,
		cancelFn:          make(map[string]context.CancelFunc),
		contextCh:         make(chan contextMessage),
	}

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
	c.workqueue.AddRateLimited(key)
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting restarter controller")

	// Start listening to context start messages
	klog.Info("Starting to listen for context start messages")
	go c.handleContextMessages(stopCh)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.hasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch workers to process VPA resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// handleContextMessages will close any old context associated with the key from the message
// and register the new cancel function for the future.
// This is to ensure that there is only one active context for any given key.
func (c *Controller) handleContextMessages(stopCh <-chan struct{}) {
	defer close(c.contextCh)

	for {
		select {
		case <-stopCh:
			klog.Info("Received stop signal. Stopping the handling of context messages.")
			return
		case cmsg := <-c.contextCh:
			oldCancelFn, ok := c.cancelFn[cmsg.key]
			if cmsg.cancelFn != nil {
				klog.Infof("Registering the context for the key: %s", cmsg.key)
				c.cancelFn[cmsg.key] = cmsg.cancelFn
			} else {
				klog.Infof("Unregistering the context for the key: %s", cmsg.key)
				delete(c.cancelFn, cmsg.key)
			}

			if ok && oldCancelFn != nil {
				klog.Infof("Cancelling older context for the key: %s", cmsg.key)
				oldCancelFn()
			}
		}
	}
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

		if err := c.processEndpoint(key); err != nil {

			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %v, requeuing", key, err)
		}

		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) processEndpoint(key string) error {
	klog.Infof("Processing endpoint: %s", key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	if namespace != c.serviceDependants.Namespace || name != c.serviceDependants.Service {
		return nil
	}

	ep, err := c.clientset.CoreV1().Endpoints(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		// The endpoint resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("endpoint '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	if !IsReadyEndpointPresentInSubsets(ep.Subsets) {
		klog.Infof("Endpoint %s does not have any endpoint subset. Skipping pod terminations.", ep.Name)
		// Cancel any existing context to pro-actively avoid shooting pods accidentally.
		c.contextCh <- contextMessage{
			key:      key,
			cancelFn: nil,
		}
		return nil
	}

	go func() {
		klog.Infof("Watching for pods in CrashLoopBackOff for a period of %s", c.watchDuration.String())
		ctx, cancelFn := context.WithTimeout(context.Background(), c.watchDuration)
		defer cancelFn()

		c.contextCh <- contextMessage{
			key:      key,
			cancelFn: cancelFn,
		}

		c.shootPodsIfNecessary(ctx, namespace)
		select {
		case <-ctx.Done():
			c.contextCh <- contextMessage{
				key:      key,
				cancelFn: nil,
			}
			return
		case <-c.stopCh:
			return
		}
	}()
	return nil
}

func (c *Controller) shootPodsIfNecessary(ctx context.Context, namespace string) error {
	for _, dependantPod := range c.serviceDependants.Dependants {
		go func(depPods dependantPods) {
			err := c.shootDependentPodsIfNecessary(ctx, namespace, &depPods)
			if err != nil {
				klog.Errorf("Error processing dependents pods: %s", err)
			}
		}(dependantPod)
	}
	return nil
}

func (c *Controller) shootDependentPodsIfNecessary(ctx context.Context, namespace string, depPods *dependantPods) error {
	selector, err := metav1.LabelSelectorAsSelector(depPods.Selector)
	if err != nil {
		return fmt.Errorf("error converting label selector to selector %s", depPods.Selector.String())
	}

	for {
		retry, err := func() (bool, error) {
			klog.Infof("Watching pods in CrashloopBackoff using selector: %s", selector.String())
			w, err := c.clientset.CoreV1().Pods(namespace).Watch(metav1.ListOptions{
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
						err := c.processPod(pod)
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

func (c *Controller) processPod(pod *v1.Pod) error {
	// Validate pod status again before shoot it out.
	po, err := c.clientset.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("error getting pod %s", pod.Name)
	}
	if !ShouldDeletePod(po) {
		return nil
	}
	klog.Infof("Deleting pod: %v", po.Name)

	return c.clientset.CoreV1().Pods(po.Namespace).Delete(po.Name, &metav1.DeleteOptions{})
}
