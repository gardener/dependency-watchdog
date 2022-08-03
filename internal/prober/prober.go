package prober

import (
	"context"
	papi "github.com/gardener/dependency-watchdog/api/prober"
	"time"

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
	scaler              DeploymentScaler
	shootclientCreator  ShootClientCreator
	internalProbeStatus probeStatus
	externalProbeStatus probeStatus
	ctx                 context.Context
	cancelFn            context.CancelFunc
	l                   logr.Logger
}

// NewProber creates a new Prober
func NewProber(parentCtx context.Context, namespace string, config *papi.Config, ctrlClient client.Client, scaler DeploymentScaler, shootClientCreator ShootClientCreator, logger logr.Logger) *Prober {
	ctx, cancelFn := context.WithCancel(parentCtx)
	return &Prober{
		namespace:          namespace,
		config:             config,
		client:             ctrlClient,
		scaler:             scaler,
		shootclientCreator: shootClientCreator,
		ctx:                ctx,
		cancelFn:           cancelFn,
		l:                  logger,
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
	_ = util.SleepWithContext(p.ctx, *p.config.InitialDelay)
	wait.JitterUntilWithContext(p.ctx, p.probe, *p.config.ProbeInterval, *p.config.BackoffJitterFactor, true)
}

func (p *Prober) probe(ctx context.Context) {
	internalShootClient, err := p.setupProbeClient(ctx, p.namespace, p.config.InternalKubeConfigSecretName)
	if err != nil {
		p.l.Error(err, "failed to create shoot client using internal secret, ignoring error, internal probe will be re-attempted", "namespace", p.namespace)
		return
	}
	p.probeInternal(internalShootClient)
	if p.internalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
		externalShootClient, err := p.setupProbeClient(ctx, p.namespace, p.config.ExternalKubeConfigSecretName)
		if err != nil {
			p.l.Error(err, "failed to create shoot client using external secret, ignoring error, probe will be re-attempted", "namespace", p.namespace)
			return
		}
		p.probeExternal(externalShootClient)
		// based on the external probe result it will either scale up or scale down
		if p.externalProbeStatus.isUnhealthy(*p.config.FailureThreshold) {
			p.l.V(4).Info("external probe is un-healthy, checking if scale down is already done or is still pending", "namespace", p.namespace)
			err := p.scaler.ScaleDown(ctx)
			if err != nil {
				p.l.Error(err, "failed to scale down resources", "namespace", p.namespace)
			}
			return
		}
		if p.externalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
			p.l.V(4).Info("external probe is healthy, checking if scale up is already done or is still pending", "namespace", p.namespace)
			err := p.scaler.ScaleUp(ctx)
			if err != nil {
				p.l.Error(err, "failed to scale up resources", "namespace", p.namespace)
			}
		}
	} else {
		p.l.V(4).Info("internal probe is not healthy, skipping external probe check and subsequent scaling", "namespace", p.namespace)
	}
}

func (p *Prober) setupProbeClient(ctx context.Context, namespace string, kubeConfigSecretName string) (kubernetes.Interface, error) {
	shootClient, err := p.shootclientCreator.CreateClient(ctx, namespace, kubeConfigSecretName, *p.config.ProbeTimeout)
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
			p.internalProbeStatus.recordFailure(err, *p.config.FailureThreshold, *p.config.InternalProbeFailureBackoffDuration)
			p.l.Error(err, "recording internal probe failure", "failedAttempts", p.internalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
		} else {
			p.internalProbeStatus.handleIgnorableError(err)
			p.l.Error(err, "internal probe was not successful. ignoring this error, will retry probe", "namespace", p.namespace)
		}
		return
	}
	p.internalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	p.l.V(4).Info("internal probe is successful", "namespace", p.namespace, "successfulAttempts", p.internalProbeStatus.successCount, "successThreshold", p.config.SuccessThreshold)
}

func (p *Prober) probeExternal(shootClient kubernetes.Interface) {
	backOffIfNeeded(&p.externalProbeStatus)
	err := p.doProbe(shootClient)
	if err != nil {
		if !p.externalProbeStatus.canIgnoreProbeError(err) {
			p.externalProbeStatus.recordFailure(err, *p.config.FailureThreshold, 0)
			p.l.Error(err, "recording external probe failure", "failedAttempts", p.externalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
			return
		}
		p.externalProbeStatus.handleIgnorableError(err)
		p.l.Error(err, "external probe was not successful. ignoring this error, will retry probe", "namespace", p.namespace)
		return
	}
	p.externalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	p.l.V(4).Info("external probe is successful", "namespace", p.namespace, "successfulAttempts", p.internalProbeStatus.successCount)
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
