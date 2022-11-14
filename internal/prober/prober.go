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
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	dwdScaler "github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/gardener/dependency-watchdog/internal/util"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultGetSecretBackoff             = 100 * time.Millisecond
	defaultGetSecretMaxAttempts         = 3
	backOffDurationForThrottledRequests = 10 * time.Second
)

// Prober represents a probe to the Kube ApiServer of a shoot
type Prober struct {
	namespace           string
	config              *papi.Config
	client              client.Client
	scaler              dwdScaler.Scaler
	shootClientCreator  ShootClientCreator
	internalProbeStatus probeStatus
	externalProbeStatus probeStatus
	ctx                 context.Context
	cancelFn            context.CancelFunc
	l                   logr.Logger
}

// NewProber creates a new Prober
func NewProber(parentCtx context.Context, namespace string, config *papi.Config, ctrlClient client.Client, scaler dwdScaler.Scaler, shootClientCreator ShootClientCreator, logger logr.Logger) *Prober {
	pLogger := logger.WithValues("shootNamespace", namespace)
	ctx, cancelFn := context.WithCancel(parentCtx)
	return &Prober{
		namespace:          namespace,
		config:             config,
		client:             ctrlClient,
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
	internalShootClient, err := p.setupProbeClient(ctx, p.namespace, p.config.InternalKubeConfigSecretName)
	if err != nil {
		p.l.Error(err, "Failed to create shoot client using internal secret, ignoring error, internal probe will be re-attempted")
		return
	}
	p.probeInternal(internalShootClient)
	if p.internalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
		externalShootClient, err := p.setupProbeClient(ctx, p.namespace, p.config.ExternalKubeConfigSecretName)
		if err != nil {
			p.l.Error(err, "Failed to create shoot client using external secret, ignoring error, probe will be re-attempted")
			return
		}
		p.probeExternal(externalShootClient)
		// based on the external probe result it will either scale up or scale down
		if p.externalProbeStatus.isUnhealthy(*p.config.FailureThreshold) {
			p.l.Info("External probe is un-healthy, checking if scale down is already done or is still pending")
			err := p.scaler.ScaleDown(ctx)
			if err != nil {
				p.l.Error(err, "Failed to scale down resources")
			}
			return
		}
		if p.externalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
			p.l.Info("External probe is healthy, checking if scale up is already done or is still pending")
			err := p.scaler.ScaleUp(ctx)
			if err != nil {
				p.l.Error(err, "Failed to scale up resources")
			}
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

func (p *Prober) probeInternal(shootClient kubernetes.Interface) {
	backOffIfNeeded(&p.internalProbeStatus)
	err := p.doProbe(shootClient)
	if err != nil {
		if !p.internalProbeStatus.canIgnoreProbeError(err) {
			p.internalProbeStatus.recordFailure(err, *p.config.FailureThreshold, p.config.InternalProbeFailureBackoffDuration.Duration)
			p.l.Info("Recording internal probe failure, Skipping external probe and scaling", "err", err.Error(), "failedAttempts", p.internalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
		} else {
			p.internalProbeStatus.handleIgnorableError(err)
			p.l.Info("Internal probe was not successful. ignoring this error", "err", err.Error())
		}
		return
	}
	p.internalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	p.l.Info("Internal probe is successful", "successfulAttempts", p.internalProbeStatus.successCount, "successThreshold", p.config.SuccessThreshold)
}

func (p *Prober) probeExternal(shootClient kubernetes.Interface) {
	backOffIfNeeded(&p.externalProbeStatus)
	err := p.doProbe(shootClient)
	if err != nil {
		if !p.externalProbeStatus.canIgnoreProbeError(err) {
			p.externalProbeStatus.recordFailure(err, *p.config.FailureThreshold, 0)
			p.l.Info("Recording external probe failure", "err", err.Error(), "failedAttempts", p.externalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
			return
		}
		p.externalProbeStatus.handleIgnorableError(err)
		p.l.Info("External probe was not successful. ignoring this error", "err", err.Error())
		return
	}
	p.externalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	p.l.Info("External probe is successful", "successfulAttempts", p.externalProbeStatus.successCount, "successThreshold", p.config.SuccessThreshold)
}

func backOffIfNeeded(ps *probeStatus) {
	if ps.backOff != nil {
		<-ps.backOff.C
		ps.backOff.Stop()
		ps.backOff = nil
	}
}

func (p *Prober) doProbe(client kubernetes.Interface) error {
	_, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	return nil
}
