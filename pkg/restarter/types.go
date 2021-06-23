// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package restarter

import (
	"time"

	"github.com/gardener/dependency-watchdog/pkg/multicontext"
	"github.com/gardener/dependency-watchdog/pkg/restarter/api"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	componentbaseconfig "k8s.io/component-base/config/v1alpha1"
)

const (
	crashLoopBackOff = "CrashLoopBackOff"
)

// Controller looks at ServiceDependants and reconciles the dependantPods once the service becomes available.
type Controller struct {
	clientset         kubernetes.Interface
	informerFactory   informers.SharedInformerFactory
	endpointInformer  cache.SharedIndexInformer
	endpointLister    listerv1.EndpointsLister
	workqueue         workqueue.RateLimitingInterface
	hasSynced         cache.InformerSynced
	stopCh            <-chan struct{}
	serviceDependants *api.ServiceDependants
	watchDuration     time.Duration
	// LeaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfig.LeaderElectionConfiguration
	*multicontext.Multicontext
}
