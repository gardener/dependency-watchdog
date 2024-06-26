// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"context"
	"github.com/gardener/dependency-watchdog/internal/prober/errors"
	"github.com/gardener/dependency-watchdog/internal/prober/shoot"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slices"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	dwdScaler "github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
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
	namespace            string
	config               *papi.Config
	workerNodeConditions map[string][]string
	scaler               dwdScaler.Scaler
	seedClient           client.Client
	shootClientCreator   shoot.ClientCreator
	backOff              *time.Timer
	ctx                  context.Context
	cancelFn             context.CancelFunc
	l                    logr.Logger
	lastErr              error // this is currently used only for unit tests
}

// NewProber creates a new Prober
func NewProber(parentCtx context.Context, seedClient client.Client, namespace string, config *papi.Config, workerNodeConditions map[string][]string, scaler dwdScaler.Scaler, shootClientCreator shoot.ClientCreator, logger logr.Logger) *Prober {
	pLogger := logger.WithValues("shootNamespace", namespace)
	ctx, cancelFn := context.WithCancel(parentCtx)
	return &Prober{
		namespace:            namespace,
		config:               config,
		workerNodeConditions: workerNodeConditions,
		scaler:               scaler,
		seedClient:           seedClient,
		shootClientCreator:   shootClientCreator,
		ctx:                  ctx,
		cancelFn:             cancelFn,
		l:                    pLogger,
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
	err := p.probeAPIServer(ctx)
	if err != nil {
		p.recordError(err, errors.ErrProbeAPIServer, "Failed to probe API server")
		p.l.Info("API server probe failed, Skipping lease probe and scaling operation", "err", err.Error())
		return
	}
	p.l.Info("API server probe is successful, will conduct node lease probe")

	shootClient, err := p.setupProbeClient(ctx)
	if err != nil {
		p.recordError(err, errors.ErrSetupProbeClient, "Failed to setup probe client")
		p.l.Error(err, "Failed to create shoot client using the KubeConfig secret, ignoring error, probe will be re-attempted")
		return
	}
	candidateNodeLeases, err := p.probeNodeLeases(ctx, shootClient)
	if err != nil {
		p.recordError(err, errors.ErrProbeNodeLease, "Failed to probe node leases")
		return
	}
	if len(candidateNodeLeases) > 1 {
		p.triggerScale(ctx, candidateNodeLeases)
	} else {
		p.l.Info("skipping scaling operation as number of candidate node leases <= 1")
	}
}

func (p *Prober) recordError(err error, code errors.ErrorCode, message string) {
	p.lastErr = errors.WrapError(err, code, message)
}

func (p *Prober) triggerScale(ctx context.Context, candidateNodeLeases []coordinationv1.Lease) {
	// revive:disable:early-return
	if p.shouldPerformScaleUp(candidateNodeLeases) {
		if err := p.scaler.ScaleUp(ctx); err != nil {
			p.recordError(err, errors.ErrScaleUp, "Failed to scale up resources")
			p.l.Error(err, "Failed to scale up resources")
		}
	} else {
		p.l.Info("Lease probe failed, performing scale down operation if required")
		if err := p.scaler.ScaleDown(ctx); err != nil {
			p.recordError(err, errors.ErrScaleDown, "Failed to scale down resources")
			p.l.Error(err, "Failed to scale down resources")
		}
		return
	}
	// revive:enable:early-return
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

func (p *Prober) setupProbeClient(ctx context.Context) (client.Client, error) {
	shootClient, err := p.shootClientCreator.CreateClient(ctx, p.l, p.config.ProbeTimeout.Duration)
	if err != nil {
		return nil, err
	}
	return shootClient, nil
}

func (p *Prober) probeAPIServer(ctx context.Context) error {
	discoveryClient, err := p.shootClientCreator.CreateDiscoveryClient(ctx, p.l, p.config.ProbeTimeout.Duration)
	if err != nil {
		p.l.Error(err, "Failed to create discovery client, probe will be re-attempted")
		p.setBackOffIfThrottlingError(err)
		return err
	}
	_, err = discoveryClient.ServerVersion()
	p.setBackOffIfThrottlingError(err)
	return err
}

func (p *Prober) probeNodeLeases(ctx context.Context, shootClient client.Client) ([]coordinationv1.Lease, error) {
	nodeNames, err := p.getFilteredNodeNames(ctx, shootClient)
	if err != nil {
		return nil, err
	}
	return p.getFilteredNodeLeases(ctx, shootClient, nodeNames)
}

// getFilteredNodeNames filters nodes for which node leases should be eventually checked in the caller. This function filters out the nodes which are:
// 1. Not managed by MCM - these nodes will not be considered for lease probe.
// 2. Unhealthy (checked via node conditions) - these will not be considered for lease probe allowing MCM to replace these nodes.
// 3. If the corresponding Machine object for a node has its state set to Terminating or Failed, the node will not be considered for lease probe.
func (p *Prober) getFilteredNodeNames(ctx context.Context, shootClient client.Client) ([]string, error) {
	nodes := &corev1.NodeList{}
	if err := shootClient.List(ctx, nodes); err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list nodes, will retry probe")
		return nil, err
	}
	machines, err := p.getMachines(ctx)
	if err != nil {
		return nil, err
	}
	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		if util.IsNodeManagedByMCM(&node) &&
			util.IsNodeHealthyByConditions(&node, util.GetWorkerUnhealthyNodeConditions(&node, p.workerNodeConditions)) &&
			util.GetMachineNotInFailedOrTerminatingState(node.Name, machines) != nil {
			nodeNames = append(nodeNames, node.Name)
		}
	}
	return nodeNames, nil
}

// getMachines will retrieve all machines in the shoot namespace for which this probe is running.
func (p *Prober) getMachines(ctx context.Context) ([]v1alpha1.Machine, error) {
	machines := &v1alpha1.MachineList{}
	if err := p.seedClient.List(ctx, machines, client.InNamespace(p.namespace)); err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list machines, will retry probe")
		return nil, err
	}
	return machines.Items, nil
}

// getFilteredNodeLeases filters out node leases which are not created for given nodeNames. The nodes are filtered via getFilteredNodeNames.
// It is assumed that the node leases have the same name as the corresponding node name for which they are created.
func (p *Prober) getFilteredNodeLeases(ctx context.Context, shootClient client.Client, nodeNames []string) ([]coordinationv1.Lease, error) {
	leases := &coordinationv1.LeaseList{}
	if err := shootClient.List(ctx, leases); err != nil {
		p.setBackOffIfThrottlingError(err)
		p.l.Error(err, "Failed to list leases, will retry probe")
		return nil, err
	}

	var filteredLeases []coordinationv1.Lease
	for _, lease := range leases.Items {
		if slices.Contains(nodeNames, lease.Name) {
			// node leases have the same names as nodes
			filteredLeases = append(filteredLeases, lease)
		}
	}
	return filteredLeases, nil
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
	if err != nil && apierrors.IsTooManyRequests(err) {
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

// AreWorkerNodeConditionsStale checks if the worker node conditions are up-to-date
func (p *Prober) AreWorkerNodeConditionsStale(newWorkerNodeConditions map[string][]string) bool {
	return !reflect.DeepEqual(p.workerNodeConditions, newWorkerNodeConditions)
}
