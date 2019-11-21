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

package scaler

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"

	"github.com/gardener/dependency-watchdog/pkg/multicontext"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// NewController initializes a new K8s depencency-watchdog controller with scaler.
func NewController(clientset kubernetes.Interface,
	mapper apimeta.RESTMapper,
	scalesGetter scale.ScalesGetter,
	sharedInformerFactory informers.SharedInformerFactory,
	probeDependantsList *ProbeDependantsList,
	stopCh <-chan struct{}) *Controller {

	c := &Controller{
		client:              clientset,
		mapper:              mapper,
		scalesGetter:        scalesGetter,
		informerFactory:     sharedInformerFactory,
		nsInformer:          sharedInformerFactory.Core().V1().Namespaces().Informer(),
		nsLister:            sharedInformerFactory.Core().V1().Namespaces().Lister(),
		secretsInformer:     sharedInformerFactory.Core().V1().Secrets().Informer(),
		secretsLister:       sharedInformerFactory.Core().V1().Secrets().Lister(),
		workqueue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Namespaces"),
		stopCh:              stopCh,
		probeDependantsList: probeDependantsList,
		Multicontext:        multicontext.New(),
	}
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&c.LeaderElection)
	c.nsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueProbe,
		UpdateFunc: func(old, new interface{}) {
			newNS := new.(*v1.Namespace)
			oldNS := old.(*v1.Namespace)
			if newNS.ResourceVersion == oldNS.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			c.enqueueProbe(new)
		},
		DeleteFunc: c.enqueueProbe,
	})
	c.secretsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueProbe,
		UpdateFunc: func(old, new interface{}) {
			newSecret := new.(*v1.Secret)
			oldSecret := old.(*v1.Secret)
			if newSecret.ResourceVersion == oldSecret.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			c.enqueueProbe(new)
		},
		DeleteFunc: c.enqueueProbe,
	})
	c.hasNamespacesSynced = c.nsInformer.HasSynced
	c.hasSecretsSynced = c.secretsInformer.HasSynced
	return c
}

// enqueueProbe takes an Secret resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Endpoints.
func (c *Controller) enqueueProbe(obj interface{}) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("object has no meta: %v", err))
		return
	}

	ns := meta.GetNamespace()
	if ns == "" {
		ns = meta.GetName()
	}
	c.workqueue.AddRateLimited(ns)
}

// Run will set up the handlers for the probes we are interested in,
// creating a worker for each internal and external probe.
// It will block until stopCh is closed, at which point it will shutdown the probe handlers
// while waiting to finish processing their current work items.
func (c *Controller) Run(threadiness int) error {
	defer utilruntime.HandleCrash()

	klog.Info("Starting scaler controller")

	klog.Info("Starting informer factory.")
	c.informerFactory.Start(c.stopCh)

	go c.Multicontext.Start(c.stopCh)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(c.stopCh, c.hasNamespacesSynced, c.hasSecretsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch workers to process Secret resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, c.stopCh)
	}

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

		if err := c.processNamespace(key); err != nil {

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

func (c *Controller) cancelNamespace(ns string) {
	c.Multicontext.ContextCh <- &multicontext.ContextMessage{
		Key:      ns,
		CancelFn: nil,
	}
}

func (c *Controller) processNamespace(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if namespace == "" {
		namespace = name
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	if c.probeDependantsList.Namespace != "" && namespace != c.probeDependantsList.Namespace {
		return nil
	}

	// Cancel earlier probes
	c.cancelNamespace(namespace)

	ns, err := c.client.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil || ns == nil || ns.DeletionTimestamp != nil {
		klog.Infof("Error getting namespace %s. Stopping probes for it. %s", namespace, err)
		return nil
	}

	klog.Infof("Starting probes for namespace: %s", namespace)
	ctx, cancelFn := context.WithCancel(context.Background())
	c.Multicontext.ContextCh <- &multicontext.ContextMessage{
		Key:      namespace,
		CancelFn: cancelFn,
	}

	for i := range c.probeDependantsList.Probes {
		probeDeps := &c.probeDependantsList.Probes[i]

		p := &prober{
			namespace:       namespace,
			mapper:          c.mapper,
			secretInterface: c.client.CoreV1().Secrets(namespace),
			scaleInterface:  c.scalesGetter.Scales(namespace),
			probeDeps:       probeDeps,
		}

		go func() {
			klog.Infof("Starting the probe in the namespace %s: %v", namespace, p.probeDeps)
			err := p.run(ctx.Done())
			if err != nil {
				klog.Errorf("Probe for namespace %s returned error: %s, %v", namespace, err, p.probeDeps)
			}
			klog.Infof("Finished the probe in the namespace %s: %v", namespace, p.probeDeps)
		}()
	}

	return nil
}
