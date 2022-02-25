// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/gardener/dependency-watchdog/pkg/scaler/api"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardenerlisterv1alpha1 "github.com/gardener/gardener/pkg/client/extensions/listers/extensions/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingapi "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	listerappsv1 "k8s.io/client-go/listers/apps/v1"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

type probeType int

const (
	externalProbe = iota
	internalProbe

	defaultInitialDelaySeconds = 30
	defaultPeriodSeconds       = 10
	defaultTimeoutSeconds      = 10
	defaultSuccessThreshold    = 1
	defaultFailureThreshold    = 3
	defaultMaxRetries          = 3
	defaultJitterMaxFactor     = 0.2
	defaultJitterSliding       = true

	kindDeployment = "Deployment"
)

type prober struct {
	namespace         string
	mapper            apimeta.RESTMapper
	secretLister      listerv1.SecretLister
	clusterLister     gardenerlisterv1alpha1.ClusterLister
	deploymentsLister listerappsv1.DeploymentLister
	scaleInterface    scale.ScaleInterface
	probeDeps         *api.ProbeDependants
	initialDelay      time.Duration
	initialDelayTimer *time.Timer
	successThreshold  int32
	failureThreshold  int32
	internalSHA       []byte
	externalSHA       []byte
	internalClient    kubernetes.Interface
	externalClient    kubernetes.Interface
	internalResult    probeResult
	externalResult    probeResult
	resultCh          chan *probeResult
}

type probeResult struct {
	lastError error
	resultRun int32
}

// getClientFromSecret constructs a Kubernetes client based on the supplied secret and
// return it along with the SHA256 checksum of the kubeconfig in the secret
// but only if the SHA256 checksum of the kubeconfig in the secret differs from oldSHA.
func (p *prober) getClientFromSecret(secretName string, oldSHA []byte) (kubernetes.Interface, []byte, error) {
	var (
		secret *corev1.Secret
		err    error
	)

	dwdGetTargetFromCacheTotal.With(prometheus.Labels{labelResource: resourceSecrets}).Inc()

	snl := p.secretLister.Secrets(p.namespace)
	for i := 0; i < defaultMaxRetries; i++ {
		secret, err = snl.Get(secretName)
		if apierrors.IsNotFound(err) {
			return nil, nil, err
		}
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, nil, err
	}

	kubeconfig := secret.Data["kubeconfig"]
	if kubeconfig == nil {
		return nil, nil, errors.New("Invalid empty kubeconfig")
	}

	newSHAArr := (sha256.Sum256(kubeconfig))
	newSHA := newSHAArr[:]
	if reflect.DeepEqual(oldSHA, newSHA) {
		return nil, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secret"}, secretName)
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, newSHA, err
	}

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, newSHA, err
	}

	config.Timeout = toDuration(p.probeDeps.Probe.TimeoutSeconds, defaultTimeoutSeconds)

	client, err := kubernetes.NewForConfig(config)
	return client, newSHA, err
}

func (p *prober) shootNotReady() (bool, error) {
	// The name of cluster is same as shoot's namespace
	clusterName := p.namespace
	cluster, err := p.clusterLister.Get(clusterName)
	if err != nil {
		klog.Errorf("Error getting cluster: %s, Err: %s", clusterName, err.Error())
		return true, err
	}
	decoder, err := extensionscontroller.NewGardenDecoder()
	if err != nil {
		klog.Errorf("Error getting gardener decoder. Cluster: %s, Err: %s", clusterName, err.Error())
		return true, err
	}

	shoot, err := extensionscontroller.ShootFromCluster(decoder, cluster)
	if err != nil {
		klog.Errorf("Error extracting shoot from cluster. Cluster: %s, Err: %s", clusterName, err.Error())
		return true, err
	}
	if (shoot.Spec.Hibernation == nil || shoot.Spec.Hibernation.Enabled == nil || *shoot.Spec.Hibernation.Enabled == false) && shoot.Status.IsHibernated == false {
		return false, nil
	}
	return true, nil
}

// tryAndRun runs a fresh prober only if either of the internal or external secrets changed and the shoot is in ready state.
// It updates the client and SHA checksums in the prober if a fresh prober was indeed run.
// It calls the prepareRun callback function to stop previous prober if any and to create a
// fresh context or stop channel for the fresh prober.
// prepareRun is called only if a fresh prober is run.
// It is a blocking function. When it returns, it cancels the probe, and removes the namespace key from probers memory map
func (p *prober) tryAndRun(prepareRun func() (stopCh <-chan struct{}), cancelFn func(), enqueueFn func(), probeRunningFn func() bool) error {
	if p == nil || p.probeDeps == nil || p.probeDeps.Probe == nil {
		return errors.New("Invalid empty probe dependants configuration")
	}
	if p.probeDeps.Probe.External == nil {
		return errors.New("Invalid empty external probe configuration")
	}
	if p.probeDeps.Probe.Internal == nil {
		return errors.New("Invalid empty internal probe configuration")
	}

	internalClient, internalSHA, internalErr := p.getClientFromSecret(p.probeDeps.Probe.Internal.KubeconfigSecretName, p.internalSHA)
	externalClient, externalSHA, externalErr := p.getClientFromSecret(p.probeDeps.Probe.External.KubeconfigSecretName, p.externalSHA)
	klog.V(4).Infof("Secret fetch completed with internalErr: %v", internalErr)
	klog.V(4).Infof("Secret fetch completed with externalErr: %v", externalErr)

	shootNotReady, err := p.shootNotReady()
	if (shootNotReady && err == nil) || apierrors.IsNotFound(internalErr) || apierrors.IsNotFound(externalErr) {
		klog.V(4).Infof("Cluster not ready: %v, internal err %v, external err %v", shootNotReady, internalErr, externalErr)
		// If shoot is not ready or secrets are not found, cancel any probe that might be running
		// No need to enqueqe; the key will be enqueued again when any of the above condition changes anyway
		cancelFn()
		if internalErr != nil {
			return internalErr
		}

		return externalErr
	}

	if err != nil || (internalErr != nil && !apierrors.IsAlreadyExists(internalErr)) || (externalErr != nil && !apierrors.IsAlreadyExists(externalErr)) {
		// There is an error, and it is not "AlreadyExists" - cancel running probe and requeue
		klog.V(4).Infof("Cluster err %v, internal err %v, external err %v", err, internalErr, externalErr)
		cancelFn()
		enqueueFn()
		if err != nil {
			return err
		}
		if internalErr != nil && !apierrors.IsAlreadyExists(internalErr) {
			return internalErr
		}

		return externalErr
	}

	if apierrors.IsAlreadyExists(internalErr) && apierrors.IsAlreadyExists(externalErr) && probeRunningFn() {
		// No change in kubeconfig. Probe is already running. No need to restart probe
		return internalErr
	}

	// If we are here, then either secrets were created/updated, or the cluster woke up from hibernation
	// Run a fresh prober goroutine
	stopCh := prepareRun()
	// This will also delete prober from memory map if present
	defer cancelFn()

	// prepareRun should have stopped previous prober goroutine.
	// So, there is no need for any synchronization here.
	p.internalClient = internalClient
	p.internalSHA = internalSHA
	p.externalClient = externalClient
	p.externalSHA = externalSHA

	return p.run(stopCh)
}

// run actually runs the prober logic. It is a blocking function. It should be called only via tryAndRun.
func (p *prober) run(stopCh <-chan struct{}) error {
	p.initialDelay = toDuration(p.probeDeps.Probe.InitialDelaySeconds, defaultInitialDelaySeconds)

	if p.probeDeps.Probe.SuccessThreshold != nil {
		p.successThreshold = *p.probeDeps.Probe.SuccessThreshold
	} else {
		p.successThreshold = defaultSuccessThreshold
	}

	if p.probeDeps.Probe.FailureThreshold != nil {
		p.failureThreshold = *p.probeDeps.Probe.FailureThreshold
	} else {
		p.failureThreshold = defaultFailureThreshold
	}

	dwdProbersTotal.With(nil).Inc()

	var err error
	d := toDuration(p.probeDeps.Probe.PeriodSeconds, defaultPeriodSeconds)
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	wait.JitterUntilWithContext(ctx, func(ctx context.Context) {
		select {
		case <-stopCh:
			return
		default:
			if p.initialDelayTimer != nil {
				<-p.initialDelayTimer.C
				p.initialDelayTimer.Stop()
				p.initialDelayTimer = nil
			}
			if err = p.probe(ctx); err != nil {
				cancelFn()
				return
			}
		}
	}, d, defaultJitterMaxFactor, defaultJitterSliding)

	return err
}

func toDuration(seconds *int32, defaultSeconds int32) time.Duration {
	if seconds != nil {
		return time.Duration(*seconds) * time.Second
	}
	return time.Duration(defaultSeconds) * time.Second
}

func (p *prober) isHealthy(pr *probeResult) bool {
	return pr.lastError == nil && pr.resultRun >= p.successThreshold
}

func (p *prober) isUnhealthy(pr *probeResult) bool {
	return pr.lastError != nil && pr.resultRun >= p.failureThreshold
}

func (p *prober) getProbeResultLabels(pr *probeResult) prometheus.Labels {
	labels := prometheus.Labels{}
	if pr.lastError != nil {
		labels[labelResult] = resultFailure
	} else {
		labels[labelResult] = resultSuccess
	}

	return labels
}

// probe probes the internal and external endpoints scales the dependents
// according to the following logic.
// 1. A probe (internal or external) is considered HEALTHY only if the last
// at least successThreshold number of consecutive attempts at that probe succeeded.
// 2. A probe (internal or external) is considered UNHEALTHY only if the last
// at least failureThreshold number of consecutive attempts at that probe failed.
// 3. A probe (internal or external) could be neither HEALTHY nor UNHEALTHY.
// 4. Everytime the internal probe transitions (from UNHEALTHY or unknown) to HEALTHY,
// no external probes are done until time has elapsed by at least initialDelay. Also,
// no actions are taken on the dependants.
// 5. Unless the internal probe is HEALTHY, no external probes are done. Also,
// no actions are taken on the dependants.
// 6. If the external probe is HEALTHY then the dependants are scaled up.
// 7. If the external probe is UNHEALTHY then the dependants are scaled down.
func (p *prober) probe(ctx context.Context) error {
	p.doProbe(fmt.Sprintf("%s/%s/internal", p.probeDeps.Name, p.namespace), p.internalClient, &p.internalResult)
	dwdInternalProbesTotal.With(p.getProbeResultLabels(&p.internalResult)).Inc()

	if p.isUnhealthy(&p.internalResult) {
		klog.V(3).Infof("%s/%s/internal is unhealthy. Activating initial delay.", p.probeDeps.Name, p.namespace)
		if p.initialDelayTimer != nil {
			p.initialDelayTimer.Stop()
		}
		p.initialDelayTimer = time.NewTimer(p.initialDelay)
		return nil // Short-circuit external probe if the internal one fails
	}

	if !p.isHealthy(&p.internalResult) {
		klog.V(3).Infof("%s/%s/internal is not healthy. Skipping the external probe.", p.probeDeps.Name, p.namespace)
		return nil //  Short-circuit external probe if the internal one fails
	}

	if p.initialDelayTimer != nil {
		p.initialDelayTimer.Stop()
		p.initialDelayTimer = nil
	}

	p.doProbe(fmt.Sprintf("%s/%s/external", p.probeDeps.Name, p.namespace), p.externalClient, &p.externalResult)
	dwdExternalProbesTotal.With(p.getProbeResultLabels(&p.externalResult)).Inc()

	if p.isHealthy(&p.externalResult) {
		return p.scaleUp(ctx)
	}
	if p.isUnhealthy(&p.externalResult) {
		return p.scaleDown(ctx)
	}

	return nil
}

func (p *prober) doProbe(msg string, client kubernetes.Interface, pr *probeResult) {
	var (
		err        error
		maxRetries = 1 // override defaultMaxRetries
	)
	for i := 0; i < maxRetries; i++ {
		if _, err = client.Discovery().ServerVersion(); err == nil {
			klog.V(4).Infof("%s: probe succeeded", msg)
			break
		}
		klog.V(3).Infof("%s: probe failed with error: %s. Will retry...", msg, err)
	}

	if (err == nil && pr.lastError != nil) || (err != nil && pr.lastError == nil) {
		pr.resultRun = 0
	}

	pr.lastError = err
	if pr.resultRun <= p.successThreshold || pr.resultRun <= p.failureThreshold { // Prevents overflow
		pr.resultRun++
	}

	klog.V(4).Infof("%s: probe result: %#v", msg, pr)
}

func retry(msg string, fn func() error, retries int) error {
	var err error
	for ; retries > 0; retries-- {
		err = fn()
		if err == nil {
			return nil
		}
		klog.Warningf("%s: %s. %d retries remaining...", msg, err, retries)
	}

	return err
}

func (p *prober) scaleTo(parentContext context.Context, msg string, replicas int32, checkFn func(oReplicas, nReplicas int32) bool) error {
	timeout := toDuration(p.probeDeps.Probe.TimeoutSeconds, defaultTimeoutSeconds)
	for _, dsd := range p.probeDeps.DependantScales {
		if dsd == nil {
			continue
		}

		ds := dsd.ScaleRef
		if replicas > 0 && dsd.Replicas != nil {
			replicas = *dsd.Replicas
		}

		prefix := fmt.Sprintf("%s: %s.%s/%s", msg, ds.APIVersion, ds.Kind, ds.Name)

		klog.V(4).Infof("%s: replicas=%d: in progress...", prefix, replicas)

		// if possible check from the cache if the target needs to be scaled
		if ds.APIVersion == appsv1.SchemeGroupVersion.String() && ds.Kind == kindDeployment {
			dwdGetTargetFromCacheTotal.With(prometheus.Labels{labelResource: resourceDeployments}).Inc()

			if d, err := p.deploymentsLister.Deployments(p.namespace).Get(ds.Name); err != nil {
				klog.Errorf("%s: Could not find the target reference: %s", prefix, err)
				klog.Errorf("%s: replicas=%d: failed", prefix, replicas)
			} else {
				var specReplicas = int32(0)
				if d.Spec.Replicas != nil {
					specReplicas = *(d.Spec.Replicas)
				}
				if !checkFn(specReplicas, replicas) {
					klog.V(4).Infof("%s: skipped because desired=%d and current=%d", prefix, replicas, specReplicas)
					continue
				}
			}
		}

		// load the target scale subresource
		// TODO avoid the second get
		gv, err := schema.ParseGroupVersion(ds.APIVersion)
		if err != nil {
			return err
		}

		gk := schema.GroupKind{
			Group: gv.Group,
			Kind:  ds.Kind,
		}

		dwdScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbDiscovery}).Inc()
		ms, err := p.mapper.RESTMappings(gk)
		if err != nil {
			if isRateLimited(err) {
				dwdThrottledScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbDiscovery}).Inc()
			}
			return err
		}

		var (
			gr schema.GroupResource
			s  *autoscalingapi.Scale
		)
		for _, m := range ms {
			gr = m.Resource.GroupResource()
			_, cancelFn := context.WithTimeout(parentContext, timeout)
			s, err = p.scaleInterface.Get(gr, ds.Name)
			cancelFn()

			dwdScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbGet}).Inc()

			if err != nil {
				klog.Errorf("%s: error getting %v: %s", prefix, gr, err)
				if isRateLimited(err) {
					dwdThrottledScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbGet}).Inc()
				}
			}
		}

		if err == nil {
			if !checkFn(s.Spec.Replicas, replicas) {
				klog.V(4).Infof("%s: skipped because desired=%d and current=%d", prefix, replicas, s.Spec.Replicas)
				continue
			}
			/*
				Check if the scaled objects has defined any delays for the operation.
				scaleUpDelay is the delay in seconds to wait before initiating scaleUp to ensures that the resource is scaled up after allowing sufficient time for system to recover.
				scaleDownDelay is the delay in seconds to wait before initiating scaleDown to ensure that the resource is scaled down after allowing its dependents room to react.
			*/
			var depChecked bool
			// Check for scaleUp delays
			if replicas > 0 {
				if dsd.ScaleUpDelaySeconds != nil {
					klog.V(4).Infof("Delaying scale up of %s by %d seconds \n", dsd.ScaleRef.Name, *dsd.ScaleUpDelaySeconds)
					time.Sleep(toDuration(dsd.ScaleUpDelaySeconds, 0))
				}
				depChecked = p.checkScaleRefDependsOn(parentContext, fmt.Sprintf("Checking dependents of %s before scaleUp", dsd.ScaleRef.Name), dsd.ScaleRefDependsOn, replicas, checkFn)
				klog.V(4).Infof("Check for Scaleref depends on returned %t\n", depChecked)

			} else if replicas == 0 { // check for scaleDown delays
				if dsd.ScaleDownDelaySeconds != nil {
					klog.V(4).Infof("Delaying scale down of %s by %d seconds \n", dsd.ScaleRef.Name, *dsd.ScaleDownDelaySeconds)
					time.Sleep(toDuration(dsd.ScaleDownDelaySeconds, 0))
				}
				depChecked = p.checkScaleRefDependsOn(parentContext, fmt.Sprintf("Checking dependents of %s before scaleDown", dsd.ScaleRef.Name), dsd.ScaleRefDependsOn, replicas, checkFn)
				klog.V(4).Infof("Check for Scaleref depends on returned %t\n", depChecked)

			} else {
				klog.Errorf("%s: Replicas has a unsupported value %d\n", prefix, replicas)
			}
			if depChecked {

				if err = retry(msg, p.getScalingFn(parentContext, gr, s, replicas), defaultMaxRetries); err != nil {
					klog.Errorf("%s: Error scaling : %s", prefix, err)
				}
				klog.Infof("%s: replicas=%d: successful", prefix, replicas)
			} else {
				klog.V(4).Infof("Check for dependents returned false. Skipping scaling")
			}
		} else {
			klog.Errorf("%s: Could not get target reference: %s", prefix, err)
			klog.Errorf("%s: replicas=%d: failed", prefix, replicas)
		}
	}

	return nil
}

func (p *prober) getScalingFn(parentContext context.Context, gr schema.GroupResource, s *autoscalingapi.Scale, replicas int32) func() error {
	return func() error {
		s = s.DeepCopy()
		s.Spec.Replicas = replicas

		timeout := toDuration(p.probeDeps.Probe.TimeoutSeconds, defaultTimeoutSeconds)
		_, cancelFn := context.WithTimeout(parentContext, timeout)
		defer cancelFn()

		dwdScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbUpdate}).Inc()

		_, err := p.scaleInterface.Update(gr, s)

		if err != nil && isRateLimited(err) {
			dwdThrottledScaleRequestsTotal.With(prometheus.Labels{labelVerb: verbGet}).Inc()
		}

		return err
	}
}

func (p *prober) scaleDown(ctx context.Context) error {
	return p.scaleTo(ctx, fmt.Sprintf("Scaling down dependents of %s/%s", p.probeDeps.Name, p.namespace), 0, func(o, n int32) bool {
		return o > n // scale to at most n
	})
}

func (p *prober) scaleUp(ctx context.Context) error {
	return p.scaleTo(ctx, fmt.Sprintf("Scaling up dependents of %s/%s", p.probeDeps.Name, p.namespace), 1, func(o, n int32) bool {
		return n > o // scale to at least n
	})
}

// Checks for a given resource considered for scale, if for the respecitve scale operations its dependent deployments are in desired state.
// If availableReplicas is not equal to desired then it fails the check and the scaling fo the parent is stopped
func (p *prober) checkScaleRefDependsOn(ctx context.Context, prefix string, dependsOnScaleRefs []autoscalingapi.CrossVersionObjectReference, replicas int32, checkFn func(oReplicas, nReplicas int32) bool) bool {
	// if possible check from the cache if the target needs to be scaled
	klog.V(4).Infof("Check scale for dependents with prefix %s and dependendents %v", prefix, dependsOnScaleRefs)
	if len(dependsOnScaleRefs) != 0 {
		for _, dependsOnScaleRef := range dependsOnScaleRefs {
			klog.V(4).Infof("Checking if the dependent scaleRef %v  has the desired replicas %d\n ", dependsOnScaleRef, replicas)
			if dependsOnScaleRef.APIVersion == appsv1.SchemeGroupVersion.String() && dependsOnScaleRef.Kind == kindDeployment {
				dwdGetTargetFromCacheTotal.With(prometheus.Labels{labelResource: resourceDeployments}).Inc()
				d, err := p.deploymentsLister.Deployments(p.namespace).Get(dependsOnScaleRef.Name)
				if err != nil {
					klog.Errorf("%s: Could not find the target reference for %s: %s", prefix, dependsOnScaleRef.Name, err)
					return false
				}
				var availableReplicas = int32(0)
				availableReplicas = d.Status.AvailableReplicas //check if available replicas is as desired
				if !checkFn(availableReplicas, replicas) {
					klog.V(4).Infof("%s: check for dependent %s succeeded as desired=%d and available=%d", prefix, d.Name, replicas, availableReplicas)
					return true // can continue with scale operation of the parent
				}
				klog.V(4).Infof("%s: check for dependent %s failed as desired=%d and available=%d", prefix, d.Name, replicas, availableReplicas)
				return false // stop the scale operation of parent as dependent has not yet scaled
			}

		}
	}
	klog.V(4).Infof("%s skipped as there are no dependents to process.", prefix)
	return true
}
