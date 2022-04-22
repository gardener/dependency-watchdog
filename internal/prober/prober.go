package prober

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("prober")

const (
	defaultGetSecretBackoff               = 100 * time.Millisecond
	defaultGetSecretMaxAttempts           = 3
	backOffDurationForThrottledRequests   = 10 * time.Second
	internalProbeUnhealthyBackoffDuration = 30 * time.Second
)

type Prober struct {
	namespace           string
	config              *Config
	client              client.Client
	scaler              DeploymentScaler
	shootclientCreator  ShootClientCreator
	internalProbeStatus probeStatus
	externalProbeStatus probeStatus
	stopC               <-chan struct{}
	cancelFn            context.CancelFunc
}

func NewProber(namespace string, config *Config, ctrlClient client.Client, scaler DeploymentScaler, shootClientCreator ShootClientCreator) *Prober {
	ctx, cancelFn := context.WithCancel(context.Background())
	return &Prober{
		namespace:          namespace,
		config:             config,
		client:             ctrlClient,
		scaler:             scaler,
		shootclientCreator: shootClientCreator,
		stopC:              ctx.Done(),
		cancelFn:           cancelFn,
	}
}

func (p *Prober) Close() {
	p.cancelFn()
}

// IsClosed checks if the context of the prober is cancelled or not.
func (p *Prober) IsClosed() bool {
	select {
	case <-p.stopC:
		return true
	default:
		return false
	}
}

func (p *Prober) Run() {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	wait.JitterUntilWithContext(ctx, func(ctx context.Context) {
		select {
		case <-p.stopC:
			logger.V(3).Info("stop has been called for prober", "namespace", p.namespace)
			return
		default:
			p.probe(ctx)
		}
	}, *p.config.ProbeInterval, *p.config.BackoffJitterFactor, true)
}

func (p *Prober) probe(ctx context.Context) {
	p.probeInternal(ctx)
	if p.internalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
		p.probeExternal(ctx)
		// based on the external probe result it will either scale up or scale down
		if p.externalProbeStatus.isUnhealthy(*p.config.FailureThreshold) {
			logger.V(4).Info("external probe is un-healthy, checking if scale down is already done or is still pending", "namespace", p.namespace)
			err := p.scaler.ScaleDown(ctx)
			if err != nil {
				logger.Error(err, "failed to scale down resources", "namespace", p.namespace)
			}
			return
		}
		if p.externalProbeStatus.isHealthy(*p.config.SuccessThreshold) {
			logger.V(4).Info("external probe is healthy, checking if scale up is already done or is still pending", "namespace", p.namespace)
			err := p.scaler.ScaleUp(ctx)
			if err != nil {
				logger.Error(err, "failed to scale up resources", "namespace", p.namespace)
			}
		}
	} else {
		logger.V(4).Info("internal probe is not healthy, skipping external probe check and subsequent scaling", "namespace", p.namespace)
	}
}

func (p *Prober) probeInternal(ctx context.Context) {
	backOffIfNeeded(&p.internalProbeStatus)
	shootClient, err := p.shootclientCreator.CreateClient(ctx, p.namespace, p.config.InternalKubeConfigSecretName)
	if err != nil {
		logger.Error(err, "failed to create shoot client using internal secret, ignoring error, probe will be re-attempted", "namespace", p.namespace)
		return
	}
	err = p.doProbe(shootClient)
	if err != nil {
		if !p.internalProbeStatus.canIgnoreProbeError(err) {
			p.internalProbeStatus.recordFailure(err, *p.config.FailureThreshold, internalProbeUnhealthyBackoffDuration)
			logger.Error(err, "recording internal probe failure", "failedAttempts", p.internalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
		}
		logger.Error(err, "internal probe was not successful. ignoring this error, will retry probe", "namespace", p.namespace)
		return
	}
	p.internalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	logger.V(4).Info("internal probe is successful", "namespace", p.namespace, "successfulAttempts", p.internalProbeStatus.successCount)
}

func (p *Prober) probeExternal(ctx context.Context) {
	backOffIfNeeded(&p.externalProbeStatus)
	shootClient, err := p.shootclientCreator.CreateClient(ctx, p.namespace, p.config.ExternalKubeConfigSecretName)
	if err != nil {
		logger.Error(err, "failed to create shoot client using external secret, ignoring error, probe will be re-attempted", "namespace", p.namespace)
		return
	}
	err = p.doProbe(shootClient)
	if err != nil {
		if !p.externalProbeStatus.canIgnoreProbeError(err) {
			p.externalProbeStatus.recordFailure(err, *p.config.FailureThreshold, 0)
			logger.Error(err, "recording external probe failure", "failedAttempts", p.externalProbeStatus.errorCount, "failureThreshold", p.config.FailureThreshold)
		}
		logger.Error(err, "external probe was not successful. ignoring this error, will retry probe", "namespace", p.namespace)
		return
	}
	p.externalProbeStatus.recordSuccess(*p.config.SuccessThreshold)
	logger.V(4).Info("external probe is successful", "namespace", p.namespace, "successfulAttempts", p.internalProbeStatus.successCount)
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
