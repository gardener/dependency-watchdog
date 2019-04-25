package restarter

import (
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

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
	retries int,
	backoff time.Duration,
	stopCh <-chan struct{}) *Controller {
	c := &Controller{
		clientset:         clientset,
		endpointInformer:  sharedInformerFactory.Core().V1().Endpoints().Informer(),
		endpointLister:    sharedInformerFactory.Core().V1().Endpoints().Lister(),
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Endpoints"),
		stopCh:            stopCh,
		serviceDependants: serviceDependants,
		backoff:           backoff,
		retries:           retries,
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
		return nil
	}

	for i := 0; i < c.retries; i++ {
		<-time.Tick(c.backoff)
		klog.Infof("Retrying shooting pods in CrashLoopBackOff")
		if err := c.shootPodsIfNecessary(namespace); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) shootPodsIfNecessary(namespace string) error {

	for _, dependantPod := range c.serviceDependants.Dependants {
		selector, err := metav1.LabelSelectorAsSelector(dependantPod.Selector)
		if err != nil {
			return fmt.Errorf("error converting label selector to selector %s", dependantPod.Selector.String())
		}
		klog.Infof("Selecting pods in CrashloopBackoff using selector: %s", selector.String())
		podList, err := c.clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			return fmt.Errorf("error listing pods with selector %s", selector.String())
		}
		for _, pod := range podList.Items {
			err := c.processPod(&pod)
			if err != nil {
				return fmt.Errorf("error processing pod %s: %v", pod.Name, err.Error())
			}
		}

	}
	return nil
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
