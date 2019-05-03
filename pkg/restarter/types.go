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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	CrashLoopBackOff = "CrashLoopBackOff"
)

// Controller looks at ServiceDependants and reconciles the dependantPods once the service becomes available.
type Controller struct {
	clientset         *kubernetes.Clientset
	endpointInformer  cache.SharedIndexInformer
	endpointLister    listerv1.EndpointsLister
	workqueue         workqueue.RateLimitingInterface
	hasSynced         cache.InformerSynced
	stopCh            <-chan struct{}
	serviceDependants *serviceDependants
	watchDuration     time.Duration
	cancelFn          map[string]context.CancelFunc
	contextCh         chan contextMessage
}

type serviceDependants struct {
	Service    string            `json:"service"`
	Labels     map[string]string `json:"labels"`
	Namespace  string            `json:"namespace"`
	Dependants []dependantPods   `json:"dependantpods"`
}

type dependantPods struct {
	Name     string                `json:"name,omitempty"`
	Selector *metav1.LabelSelector `json:"selector"`
}

type contextMessage struct {
	key      string
	cancelFn context.CancelFunc
}
