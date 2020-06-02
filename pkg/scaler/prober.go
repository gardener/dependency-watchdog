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
	"errors"
	"fmt"
	"reflect"
	"time"

	"crypto/sha256"

	autoscalingapi "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
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
	maxRetries                 = 3
)

type prober struct {
	namespace         string
	mapper            apimeta.RESTMapper
	secretLister      listerv1.SecretLister
	scaleInterface    scale.ScaleInterface
	probeDeps         *probeDependants
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

	snl := p.secretLister.Secrets(p.namespace)
	for i := 0; i < maxRetries; i++ {
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

// tryAndRun runs a fresh prober only if either of the internal or external secrets changed.
// It updates the client and SHA checksums in the prober if a fresh prober was indeed run.
// It calls the prepareRun callback function to stop previous prober if any and to create a
// fresh context or stop channel for the fresh prober.
// prepareRun is called only if a fresh prober is run.
// It is a blocking function.
func (p *prober) tryAndRun(prepareRun func() (stopCh <-chan struct{})) error {
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

	// Run a fresh prober goroutine if any one of the secrets changed
	if internalErr == nil || externalErr == nil {
		var stopCh = prepareRun()

		// prepareRun should have stopped previous prober goroutine.
		// So, there is no need for any synchronization here.
		p.internalClient = internalClient
		p.internalSHA = internalSHA
		p.externalClient = externalClient
		p.externalSHA = externalSHA

		return p.run(stopCh)
	}

	if internalErr != nil {
		return internalErr
	}

	return externalErr
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

	ticker := time.NewTicker(toDuration(p.probeDeps.Probe.PeriodSeconds, defaultPeriodSeconds))
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return nil
		case <-ticker.C:
			if p.initialDelayTimer != nil {
				<-p.initialDelayTimer.C
				p.initialDelayTimer.Stop()
				p.initialDelayTimer = nil
			}
			if err := p.probe(); err != nil {
				return err
			}
		}
	}
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
func (p *prober) probe() error {
	p.doProbe(fmt.Sprintf("%s/%s/internal", p.probeDeps.Name, p.namespace), p.internalClient, &p.internalResult)
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
	if p.isHealthy(&p.externalResult) {
		return p.scaleUp()
	}
	if p.isUnhealthy(&p.externalResult) {
		return p.scaleDown()
	}

	return nil
}

func (p *prober) doProbe(msg string, client kubernetes.Interface, pr *probeResult) {
	var err error
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

func (p *prober) scaleTo(msg string, replicas int32, checkFn func(oReplicas, nReplicas int32) bool) error {
	klog.V(4).Infof("%s: replicas=%d: in progress...", msg, replicas)

	timeout := toDuration(p.probeDeps.Probe.TimeoutSeconds, defaultTimeoutSeconds)
	for _, dsd := range p.probeDeps.DependantScales {
		if dsd == nil {
			continue
		}

		ds := dsd.ScaleRef
		if replicas > 0 && dsd.Replicas != nil {
			replicas = *dsd.Replicas
		}

		gv, err := schema.ParseGroupVersion(ds.APIVersion)
		if err != nil {
			return err
		}

		gk := schema.GroupKind{
			Group: gv.Group,
			Kind:  ds.Kind,
		}
		ms, err := p.mapper.RESTMappings(gk)
		if err != nil {
			return err
		}

		var (
			gr schema.GroupResource
			s  *autoscalingapi.Scale
		)
		for _, m := range ms {
			gr = m.Resource.GroupResource()
			ctx, cancelFn := context.WithTimeout(context.Background(), timeout)
			s, err = p.scaleInterface.Get(ctx, gr, ds.Name, metav1.GetOptions{})
			cancelFn()
			if err != nil {
				klog.Errorf("%s: error getting %v/%s: %s", msg, gr, ds.Name, err)
			}
		}

		if err == nil {
			if !checkFn(s.Spec.Replicas, replicas) {
				klog.V(4).Infof("%s: skipped because desired=%d and current=%d", msg, replicas, s.Spec.Replicas)
				continue
			}

			if err = retry(msg, p.getScalingFn(gr, s, replicas), maxRetries); err != nil {
				klog.Errorf("%s: Error scaling %s/%s: %s", msg, s, ds.Name, err)
			}
			klog.Infof("%s: replicas=%d: successful", msg, replicas)
		} else {
			klog.Errorf("%s: Could not find  %s: %s", msg, ds, err)
			klog.Errorf("%s: replicas=%d: failed", msg, replicas)
		}
	}

	return nil
}

func (p *prober) getScalingFn(gr schema.GroupResource, s *autoscalingapi.Scale, replicas int32) func() error {
	return func() error {
		s = s.DeepCopy()
		s.Spec.Replicas = replicas
		timeout := toDuration(p.probeDeps.Probe.TimeoutSeconds, defaultTimeoutSeconds)
		ctx, cancelFn := context.WithTimeout(context.Background(), timeout)
		defer cancelFn()

		_, err := p.scaleInterface.Update(ctx, gr, s, metav1.UpdateOptions{})
		return err
	}
}

func (p *prober) scaleDown() error {
	return p.scaleTo(fmt.Sprintf("Scaling down %s/%s", p.probeDeps.Name, p.namespace), 0, func(o, n int32) bool {
		return o > n // scale to at most n
	})
}

func (p *prober) scaleUp() error {
	return p.scaleTo(fmt.Sprintf("Scaling up %s/%s", p.probeDeps.Name, p.namespace), 1, func(o, n int32) bool {
		return n > o // scale to at least n
	})
}
