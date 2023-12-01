// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prober

import (
	"context"
	"fmt"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	dwdScaler "github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/gardener/dependency-watchdog/internal/util"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultGetSecretBackoff             = 100 * time.Millisecond
	defaultGetSecretMaxAttempts         = 3
	backOffDurationForThrottledRequests = 10 * time.Second
	// expiryBufferFraction is used to compute a revised expiry time used by the prober to determine expired leases
	// This will be used only when nodeLeaseDuration is equal to the kcmNodeMonitorGraceDuration.
	// Using a fraction allows the prober to intervene before KCM marks a node as unknown, but at the same time allowing
	// kubelet sufficient retries to renew the node lease.
	// Eg:- nodeLeaseDuration = 40s, kcmNodeMonitorGraceDuration = 40s, kubeletRenewTime = 10s.
	// 		The node lease will be considered expired by the prober at 30s. This allows kubelet 3 attempts instead of 4
	// 		to renew the node lease.
	expiryBufferFraction = 0.75
	nodeLeaseNamespace   = "kube-node-lease"
)

// Prober represents a probe to the Kube ApiServer of a shoot
type Prober struct {
	namespace          string
	config             *papi.Config
	scaler             dwdScaler.Scaler
	shootClientCreator ShootClientCreator
	backOff            *time.Timer
	ctx                context.Context
	cancelFn           context.CancelFunc
	l                  logr.Logger
}

// NewProber creates a new Prober
func NewProber(parentCtx context.Context, namespace string, config *papi.Config, scaler dwdScaler.Scaler, shootClientCreator ShootClientCreator, logger logr.Logger) *Prober {
	pLogger := logger.WithValues("shootNamespace", namespace)
	ctx, cancelFn := context.WithCancel(parentCtx)
	return &Prober{
		namespace:          namespace,
		config:             config,
		scaler:             scaler,
		shootClientCreator: shootClientCreator,
		ctx:                ctx,
		cancelFn:           cancelFn,
		l:                  pLogger,
	}
}

// Close closes a probe
func (p *Prober) Close() {
	p.cancelFn()
}

// IsClosed checks if the context of the prober is cancelled or not.
func (p *Prober) IsClosed() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// Run starts a probe which will run with a configured interval and jitter.
func (p *Prober) Run() {
	_ = util.SleepWithContext(p.ctx, p.config.InitialDelay.Duration)
	wait.JitterUntilWithContext(p.ctx, p.probe, p.config.ProbeInterval.Duration, *p.config.BackoffJitterFactor, true)
}

func (p *Prober) probe(ctx context.Context) {
	p.backOffIfNeeded()
	shootClient, err := p.setupProbeClient(ctx, p.namespace, p.config.KubeConfigSecretName)
	if err != nil {
		p.l.Error(err, "Failed to create shoot client using the KubeConfig secret, ignoring error, probe will be re-attempted")
		return
	}
	err = p.probeAPIServer(shootClient)
	if err != nil {
		p.l.Info("API server probe failed, Skipping lease probe and scaling operation", "err", err.Error())
		return
	}
	p.l.Info("API server probe is successful, will conduct node lease probe")

	shouldScale, err := p.probeNodeLeases(shootClient)
	if shouldScale {
		if err != nil {
			p.l.Info("Lease probe failed, performing scale down operation if required", "err", err.Error())
			err := p.scaler.ScaleDown(ctx)
			if err != nil {
				p.l.Error(err, "Failed to scale down resources")
			}
			return
		}
		p.l.Info("Lease probe succeeded, performing scale up operation if required")
		err := p.scaler.ScaleUp(ctx)
		if err != nil {
			p.l.Error(err, "Failed to scale up resources")
		}
	}
}

func (p *Prober) setupProbeClient(ctx context.Context, namespace string, kubeConfigSecretName string) (kubernetes.Interface, error) {
	shootClient, err := p.shootClientCreator.CreateClient(ctx, p.l, namespace, kubeConfigSecretName, p.config.ProbeTimeout.Duration)
	if err != nil {
		return nil, err
	}
	return shootClient, nil
}

func (p *Prober) probeAPIServer(shootClient kubernetes.Interface) error {
	_, err := shootClient.Discovery().ServerVersion()
	p.setBackOffIfThrottlingError(err)
	return err
}

func (p *Prober) probeNodeLeases(shootClient kubernetes.Interface) (shouldScale bool, err error) {
	leaseList, err := shootClient.CoordinationV1().Leases(nodeLeaseNamespace).List(p.ctx, metav1.ListOptions{})
	if err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list leases, will retry probe")
		return false, err
	}
	// the filtering of leases is done due to an upstream bug in k8s (https://github.com/kubernetes/kubernetes/issues/119660)
	// because of which there can be leases without owner references for already removed nodes. we need to filter out such leases.
	var ownedLeaseCount, expiredLeaseCount float64
	for _, lease := range leaseList.Items {
		if lease.OwnerReferences != nil {
			ownedLeaseCount++
			if p.isLeaseExpired(lease) {
				expiredLeaseCount++
			}
		}
	}
	if ownedLeaseCount == 0 {
		p.l.Info("No owned node leases are present in the cluster, skipping node lease probe and scaling operation")
		return false, nil
	}
	if expiredLeaseCount/ownedLeaseCount > *p.config.LeaseFailureThresholdFraction {
		return true, fmt.Errorf("leaseFailureThreshold %f breached. %f leases expired out of total %f observed node leases", *p.config.LeaseFailureThresholdFraction, expiredLeaseCount, ownedLeaseCount)
	}
	return true, nil
}

func (p *Prober) isLeaseExpired(lease coordinationv1.Lease) bool {
	revisedNodeLeaseExpiryTime := float64(p.config.KCMNodeMonitorGraceDuration.Duration) * expiryBufferFraction
	expiryTime := lease.Spec.RenewTime.Add(time.Duration(revisedNodeLeaseExpiryTime))
	return util.EqualOrBeforeNow(expiryTime)
}

func (p *Prober) backOffIfNeeded() {
	if p.backOff != nil {
		<-p.backOff.C
		p.backOff.Stop()
		p.backOff = nil
	}
}

func (p *Prober) doProbe(client kubernetes.Interface) error {
	_, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	return nil
}

func (p *Prober) setBackOffIfThrottlingError(err error) {
	if apierrors.IsTooManyRequests(err) {
		p.l.V(4).Info("API server is throttled, backing off for %v seconds", backOffDurationForThrottledRequests.Seconds())
		p.resetBackoff(backOffDurationForThrottledRequests)
	}
}

func (p *Prober) resetBackoff(d time.Duration) {
	if p.backOff != nil {
		p.backOff.Stop()
	}
	p.backOff = time.NewTimer(d)
}
