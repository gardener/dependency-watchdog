// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"context"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

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

// GetConfig returns the probe config for the prober.
func (p *Prober) GetConfig() *papi.Config {
	return p.config
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

	candidateNodeLeases, err := p.probeNodeLeases(shootClient)
	if err != nil {
		return
	}
	if p.shouldPerformScaleUp(candidateNodeLeases) {
		if err = p.scaler.ScaleUp(ctx); err != nil {
			p.l.Error(err, "Failed to scale up resources")
		}
	} else {
		p.l.Info("Lease probe failed, performing scale down operation if required")
		if err = p.scaler.ScaleDown(ctx); err != nil {
			p.l.Error(err, "Failed to scale down resources")
		}
		return
	}
}

// shouldPerformScaleUp returns true if the ratio of expired node leases to valid node leases is less than
// the NodeLeaseFailureFraction set in the prober config
func (p *Prober) shouldPerformScaleUp(candidateNodeLeases []coordinationv1.Lease) bool {
	if len(candidateNodeLeases) == 0 {
		p.l.Info("No owned node leases are present in the cluster, performing scale up operation if required")
		return true
	}
	var expiredNodeLeaseCount float64
	for _, lease := range candidateNodeLeases {
		if p.isLeaseExpired(lease) {
			expiredNodeLeaseCount++
		}
	}
	shouldScaleUp := expiredNodeLeaseCount/float64(len(candidateNodeLeases)) < *p.config.NodeLeaseFailureFraction
	if shouldScaleUp {
		p.l.Info("Lease probe succeeded, performing scale up operation if required")
	}
	return shouldScaleUp
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

func (p *Prober) probeNodeLeases(shootClient kubernetes.Interface) ([]coordinationv1.Lease, error) {
	nodes, err := shootClient.CoreV1().Nodes().List(p.ctx, metav1.ListOptions{})
	if err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list nodes, will retry probe")
		return nil, err
	}

	nodeNames := sets.New[string]()
	for _, node := range nodes.Items {
		nodeNames.Insert(node.Name)
	}

	leases, err := shootClient.CoordinationV1().Leases(nodeLeaseNamespace).List(p.ctx, metav1.ListOptions{})
	if err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list leases, will retry probe")
		return nil, err
	}

	var filteredLeases []coordinationv1.Lease
	for _, lease := range leases.Items {
		// skip leases belonging to non-existing nodes
		if nodeNames.Has(lease.Name) { // node leases have the same names as nodes
			filteredLeases = append(filteredLeases, lease)
		}
	}

	return filteredLeases, err
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
		p.l.V(4).Info("API server is throttled, backing off", "backOffDuration", backOffDurationForThrottledRequests.Seconds())
		p.resetBackoff(backOffDurationForThrottledRequests)
	}
}

func (p *Prober) resetBackoff(d time.Duration) {
	if p.backOff != nil {
		p.backOff.Stop()
	}
	p.backOff = time.NewTimer(d)
}
