package prober

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	Namespace           string
	Config              *Config
	Client              client.Client
	Scaler              DeploymentScaler
	internalProbeStatus probeStatus
	externalProbeStatus probeStatus
	backOff             *time.Timer
	stopC               <-chan struct{}
	cancelFn            context.CancelFunc
}

func NewProber(namespace string, config *Config, ctrlClient client.Client, scaler DeploymentScaler) *Prober {
	ctx, cancelFn := context.WithCancel(context.Background())
	return &Prober{
		Namespace: namespace,
		Config:    config,
		Client:    ctrlClient,
		Scaler:    scaler,
		stopC:     ctx.Done(),
		cancelFn:  cancelFn,
	}
}

func (p *Prober) Close() {
	p.cancelFn()
}

func (p *Prober) Run() {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()
	wait.JitterUntilWithContext(ctx, func(ctx context.Context) {
		select {
		case <-p.stopC:
			logger.V(3).Info("stop has been called for prober", "namespace", p.Namespace)
			return
		default:
			p.probe(ctx)
		}
	}, *p.Config.ProbeInterval, *p.Config.BackoffJitterFactor, true)
}

func (p *Prober) probe(ctx context.Context) {
	p.probeInternal(ctx)
	if p.internalProbeStatus.isHealthy(*p.Config.SuccessThreshold) {
		p.probeExternal(ctx)
		// based on the external probe result it will either scale up or scale down
		if p.externalProbeStatus.isUnhealthy(*p.Config.FailureThreshold) {
			logger.V(4).Info("external probe is un-healthy, checking if scale down is already done or is still pending", "namespace", p.Namespace)
			err := p.Scaler.ScaleDown(ctx)
			if err != nil {
				logger.Error(err, "failed to scale down resources", "namespace", p.Namespace)
			}
			return
		}
		if p.externalProbeStatus.isHealthy(*p.Config.SuccessThreshold) {
			logger.V(4).Info("external probe is healthy, checking if scale up is already done or is still pending", "namespace", p.Namespace)
			err := p.Scaler.ScaleUp(ctx)
			if err != nil {
				logger.Error(err, "failed to scale up resources", "namespace", p.Namespace)
			}
		}
	} else {
		logger.V(4).Info("internal probe is not healthy, skipping external probe check and subsequent scaling", "namespace", p.Namespace)
	}
}

func (p *Prober) probeInternal(ctx context.Context) {
	shootClient, err := p.createShootClient(ctx, p.Config.InternalKubeConfigSecretName)
	if err != nil {
		logger.Error(err, "failed to create shoot client using internal secret, ignoring error, probe will be re-attempted", "namespace", p.Namespace)
		return
	}
	err = p.doProbe(shootClient)
	if err != nil {
		if !p.internalProbeStatus.canIgnoreProbeError(err) {
			p.internalProbeStatus.recordFailure(err, *p.Config.FailureThreshold)
			logger.Error(err, "recording internal probe failure", "failedAttempts", p.internalProbeStatus.errorCount, "failureThreshold", p.Config.FailureThreshold)
		}
		logger.Error(err, "internal probe was not successful. ignoring this error, will retry probe", "namespace", p.Namespace)
		return
	}
	p.internalProbeStatus.recordSuccess(*p.Config.SuccessThreshold)
	logger.V(4).Info("internal probe is successful", "namespace", p.Namespace, "successfulAttempts", p.internalProbeStatus.successCount)
}

func (p *Prober) probeExternal(ctx context.Context) {
	shootClient, err := p.createShootClient(ctx, p.Config.ExternalKubeConfigSecretName)
	if err != nil {
		logger.Error(err, "failed to create shoot client using external secret, ignoring error, probe will be re-attempted", "namespace", p.Namespace)
		return
	}
	err = p.doProbe(shootClient)
	if err != nil {
		if !p.externalProbeStatus.canIgnoreProbeError(err) {
			p.externalProbeStatus.recordFailure(err, *p.Config.FailureThreshold)
			logger.Error(err, "recording external probe failure", "failedAttempts", p.externalProbeStatus.errorCount, "failureThreshold", p.Config.FailureThreshold)
		}
		logger.Error(err, "external probe was not successful. ignoring this error, will retry probe", "namespace", p.Namespace)
		return
	}
	p.externalProbeStatus.recordSuccess(*p.Config.SuccessThreshold)
	logger.V(4).Info("external probe is successful", "namespace", p.Namespace, "successfulAttempts", p.internalProbeStatus.successCount)
}

func (p *Prober) doProbe(client kubernetes.Interface) error {
	_, err := client.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	return nil
}

func (p *Prober) createShootClient(ctx context.Context, secretName string) (kubernetes.Interface, error) {
	operation := fmt.Sprintf("get-secret-%s-for-namespace-%s", secretName, p.Namespace)
	retryResult := util.Retry(ctx,
		operation,
		func() ([]byte, error) { return util.GetKubeConfigFromSecret(ctx, p.Namespace, secretName, p.Client) },
		defaultGetSecretMaxAttempts,
		defaultGetSecretBackoff,
		canRetrySecretGet)
	if retryResult.Err != nil {
		return nil, retryResult.Err
	}
	return util.CreateClientFromKubeConfigBytes(retryResult.Value)
}

func canRetrySecretGet(err error) bool {
	return !apierrors.IsNotFound(err)
}
