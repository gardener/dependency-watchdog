package prober

import (
	"context"
	"fmt"

	"github.com/gardener/dependency-watchdog/internal/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShootClientCreator interface {
	CreateClient(ctx context.Context, namespace string, secretName string) (kubernetes.Interface, error)
}

func NewShootClientCreator(client client.Client) ShootClientCreator {
	return &shootclientCreator{client}
}

type shootclientCreator struct {
	client.Client
}

func (s *shootclientCreator) CreateClient(ctx context.Context, namespace string, secretName string) (kubernetes.Interface, error) {
	operation := fmt.Sprintf("get-secret-%s-for-namespace-%s", secretName, namespace)
	retryResult := util.Retry(ctx,
		operation,
		func() ([]byte, error) { return util.GetKubeConfigFromSecret(ctx, namespace, secretName, s.Client) },
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
