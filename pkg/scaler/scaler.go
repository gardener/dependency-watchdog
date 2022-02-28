// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/dependency-watchdog/pkg/scaler/api"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"

	"github.com/gardener/dependency-watchdog/pkg/multicontext"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	gardenerinformers "github.com/gardener/gardener/pkg/client/extensions/informers/externalversions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	gardenerInformerFactory gardenerinformers.SharedInformerFactory,
	probeDependantsList *api.ProbeDependantsList,
	stopCh <-chan struct{}) *Controller {

	c := &Controller{
		client:                 clientset,
		mapper:                 mapper,
		scalesGetter:           scalesGetter,
		informerFactory:        sharedInformerFactory,
		secretsInformer:        sharedInformerFactory.Core().V1().Secrets().Informer(),
		secretsLister:          sharedInformerFactory.Core().V1().Secrets().Lister(),
		clusterInformerFactory: gardenerInformerFactory,
		clusterInformer:        gardenerInformerFactory.Extensions().V1alpha1().Clusters().Informer(),
		clusterLister:          gardenerInformerFactory.Extensions().V1alpha1().Clusters().Lister(),
		deploymentsInformer:    sharedInformerFactory.Apps().V1().Deployments().Informer(),
		deploymentsLister:      sharedInformerFactory.Apps().V1().Deployments().Lister(),
		workqueue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Namespaces"),
		stopCh:                 stopCh,
		probeDependantsList:    probeDependantsList,
		Multicontext:           multicontext.New(),
		LeaderElection: componentbaseconfigv1alpha1.LeaderElectionConfiguration{
			ResourceLock: resourcelock.EndpointsLeasesResourceLock,
		},
	}
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&c.LeaderElection)
	c.clusterInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			newCluster := new.(*gardenerv1alpha1.Cluster)
			oldCluster := old.(*gardenerv1alpha1.Cluster)
			klog.V(4).Infof("Update event on cluster: %v", newCluster.Name)
			if newCluster.ResourceVersion == oldCluster.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}

			if shootHibernationStateChanged(oldCluster, newCluster) {
				// namespace is same as cluster's name
				ns := newCluster.Name
				klog.V(4).Infof("Requeueing namespace: %v", ns)
				if c.probeDependantsList.Namespace != "" && ns != c.probeDependantsList.Namespace {
					// skip reconciling other namespaces if a namespace was already configured
					return
				}
				c.workqueue.AddRateLimited(ns)
				klog.V(4).Infof("Requeued namespace: %v", ns)
			} else {
				klog.V(5).Infof("Ignore update event on cluster: %v", newCluster.Name)
			}
			return
		},
	})
	c.secretsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			klog.V(4).Infof("Secret Added\n")
			c.enqueueProbe(new)
		},
		UpdateFunc: func(old, new interface{}) {
			newSecret := new.(*v1.Secret)
			oldSecret := old.(*v1.Secret)
			if newSecret.ResourceVersion == oldSecret.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			klog.V(4).Info("Secret changed")
			c.enqueueProbe(new)
		},
		DeleteFunc: func(old interface{}) {
			klog.V(4).Infof("Secret deleted\n")
			c.enqueueProbe(old)
		},
	})
	c.hasSecretsSynced = c.secretsInformer.HasSynced
	c.hasDeploymentsSynced = c.deploymentsInformer.HasSynced
	c.hasClustersSynced = c.clusterInformer.HasSynced
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
	name := meta.GetName()

	if c.probeDependantsList.Namespace != "" && ns != c.probeDependantsList.Namespace {
		// skip reconciling other namespaces if a namespace was already configured
		return
	}

	var found = false
	for i := range c.probeDependantsList.Probes {
		var probe = &(c.probeDependantsList.Probes[i])
		if probe.Probe == nil {
			continue
		}

		if probe.Probe.External != nil && probe.Probe.External.KubeconfigSecretName == name {
			found = true
			break
		}

		if probe.Probe.Internal != nil && probe.Probe.Internal.KubeconfigSecretName == name {
			found = true
			break
		}
	}

	// Enqueue for reconciliation only if the secret name is one of the names configured.
	if found {
		klog.V(4).Infof("Enque probe received in namespace %s for name %s", ns, name)
		c.workqueue.AddRateLimited(ns)
	}
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
	c.clusterInformerFactory.Start(c.stopCh)

	go c.Multicontext.Start(c.stopCh)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(c.stopCh, c.hasSecretsSynced, c.hasDeploymentsSynced, c.hasClustersSynced); !ok {
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
		klog.V(4).Infof("Received shutdown signal from worker queue\n")
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

func (c *Controller) processNamespace(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if namespace == "" {
		namespace = name
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return err
	}
	if c.probeDependantsList.Namespace != "" && namespace != c.probeDependantsList.Namespace {
		klog.V(5).Infof("Namespace %s is not in the list probe dependant namespace \n", namespace)
		return nil
	}

	for i := range c.probeDependantsList.Probes {
		probeDeps := &c.probeDependantsList.Probes[i]

		go func(ns string, pd *api.ProbeDependants) {
			p := &prober{
				namespace:         ns,
				mapper:            c.mapper,
				secretLister:      c.secretsLister,
				clusterLister:     c.clusterLister,
				deploymentsLister: c.deploymentsLister,
				scaleInterface:    c.scalesGetter.Scales(ns),
				probeDeps:         probeDeps,
			}
			err := p.tryAndRun(func() <-chan struct{} {
				klog.Infof("Starting the probe in the namespace %s: %v", ns, pd.Name)
				ctx, cancelFn := c.newContext(ns, pd)
				klog.V(5).Infof("Created the context %v with cancelFun %v\n", ctx, cancelFn)
				// Register the context's cancelFn. This also cancels the previous context if any.
				c.Multicontext.ContextCh <- &multicontext.ContextMessage{
					Key:      c.getKey(ns, pd),
					CancelFn: cancelFn,
				}

				c.registerProber(p)
				return ctx.Done()
			}, func() {
				klog.V(4).Infof("Setting the context nil for ns %s and probe dependent %v\n", ns, probeDeps)
				c.Multicontext.ContextCh <- &multicontext.ContextMessage{
					Key:      c.getKey(ns, pd),
					CancelFn: nil,
				}
			}, func() {
				klog.V(4).Infof("Enqueuing with a delay of 10 mins\n")
				c.workqueue.AddAfter(ns, 10*time.Minute)
			}, func() bool {

				_, ok := c.probers[ns]
				klog.V(4).Infof("Prober ran with ok code %v\n", ok)
				return ok
			})

			if err == nil {
				klog.Infof("Finished the probe in the namespace %s: %v", ns, pd)
			} else if apierrors.IsAlreadyExists(err) {
				klog.V(4).Infof("Probe already exists for the namespace %s: %v", ns, pd)
			} else {
				klog.Errorf("Probe for namespace %s returned error: %s, %v", ns, err, pd)
			}
		}(namespace, probeDeps)
	}

	return nil
}

func (c *Controller) getKey(ns string, probeDeps *api.ProbeDependants) string {
	return ns + "/" + probeDeps.Name
}

// registerProber registers the prober in the probers map, replacing any pre-existing registration.
func (c *Controller) registerProber(p *prober) *prober {
	var (
		ns        = p.namespace
		probeDeps = p.probeDeps
	)
	if probeDeps == nil {
		return nil
	}

	key := c.getKey(ns, probeDeps)
	klog.V(4).Infof("Registering Probe for key %s\n", key)

	c.mux.Lock() // serialize access to c.probers
	defer c.mux.Unlock()

	if c.probers == nil {
		klog.V(4).Infof("No probers are running yet. Adding key %s to the probers list\n", key)
		c.probers = make(map[string]*prober)
	}

	pb, ok := c.probers[key]
	if ok && pb != nil {
		klog.V(4).Infof("Found and existing probe for key %s so using existing probe %v \n", key, pb)
		return pb
	}

	pb = &prober{
		namespace:      ns,
		mapper:         c.mapper,
		secretLister:   c.secretsLister,
		scaleInterface: c.scalesGetter.Scales(ns),
		probeDeps:      probeDeps,
	}
	c.probers[key] = pb
	klog.V(4).Infof("Created a new probe and added for key %v \n", key)
	return pb
}

func (c *Controller) deleteProber(key string) {
	c.mux.Lock() // serialize access to c.probers
	defer c.mux.Unlock()

	if c.probers == nil {
		klog.V(4).Infof("No prober found returning\n")
		return
	}

	delete(c.probers, key)
	klog.V(4).Infof("Deleted probe for key %v \n", key)
}

func (c *Controller) newContext(ns string, probeDeps *api.ProbeDependants) (context.Context, context.CancelFunc) {
	key := c.getKey(ns, probeDeps)

	ctx, cancelFn := context.WithCancel(context.Background())
	klog.V(4).Infof("Created new context %v with cancelFn %v \n", ctx, cancelFn)
	return ctx, func() {
		defer cancelFn()
		c.deleteProber(key)
	}
}
