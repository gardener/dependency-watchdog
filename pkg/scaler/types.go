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
	"github.com/gardener/dependency-watchdog/pkg/multicontext"
	autoscaling "k8s.io/api/autoscaling/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	scale "k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	componentbaseconfig "k8s.io/component-base/config/v1alpha1"
)

// Controller looks at ServiceDependants and reconciles the dependantPods once the service becomes available.
type Controller struct {
	client              kubernetes.Interface
	mapper              apimeta.RESTMapper
	scalesGetter        scale.ScalesGetter
	informerFactory     informers.SharedInformerFactory
	nsInformer          cache.SharedIndexInformer
	nsLister            listerv1.NamespaceLister
	secretsInformer     cache.SharedIndexInformer
	secretsLister       listerv1.SecretLister
	workqueue           workqueue.RateLimitingInterface
	hasNamespacesSynced cache.InformerSynced
	hasSecretsSynced    cache.InformerSynced
	stopCh              <-chan struct{}
	probeDependantsList *ProbeDependantsList
	*multicontext.Multicontext
	// LeaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfig.LeaderElectionConfiguration
}

// ProbeDependantsList holds a list of probes (internal and external) and their corresponding
// dependant Scales. If the external probe fails and the internal probe still succeeds, then the
// corresponding dependant Scales are scaled down to `zero`. They are scaled back to their
// original scale when the external probe succeeds again.
type ProbeDependantsList struct {
	Probes    []probeDependants `json:"probes"`
	Namespace string            `json:"namespace"`
}

type probeDependants struct {
	Name            string                   `json:"name"`
	Probe           *probeConfig             `json:"probe"`
	DependantScales []*dependantScaleDetails `json:"dependantScales"`
}

type probeConfig struct {
	External            *probeDetails `json:"external,omitempty"`
	Internal            *probeDetails `json:"internal,omitempty"`
	InitialDelaySeconds *int32        `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      *int32        `json:"timeoutSeconds,omitempty"`
	PeriodSeconds       *int32        `json:"periodSeconds,omitempty"`
	SuccessThreshold    *int32        `json:"successThreshold,omitempty"`
	FailureThreshold    *int32        `json:"failureThreshold,omitempty"`
}

type probeDetails struct {
	KubeconfigSecretName string `json:"kubeconfigSecretName"`
}

type dependantScaleDetails struct {
	ScaleRef autoscaling.CrossVersionObjectReference `json:"scaleRef"`
	Replicas *int32                                  `json:"replicas"`
}
