// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package shoot

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/discovery"

	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultGetSecretBackoff     = 100 * time.Millisecond
	defaultGetSecretMaxAttempts = 3
)

// ClientCreator provides a facade to create kubernetes client targeting a shoot.
type ClientCreator interface {
	// CreateClient creates a new client.Client to connect to the Kube ApiServer running in the passed-in shoot control namespace.
	CreateClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (client.Client, error)
	// CreateDiscoveryClient creates a new discovery.DiscoveryInterface to connect to the Kube ApiServer running in the passed-in shoot control namespace.
	CreateDiscoveryClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (discovery.DiscoveryInterface, error)
}

// NewClientCreator creates an instance of ClientCreator.
func NewClientCreator(namespace string, secretName string, client client.Client) ClientCreator {
	return &clientCreator{
		namespace:  namespace,
		secretName: secretName,
		client:     client,
	}
}

type clientCreator struct {
	namespace  string
	secretName string
	client     client.Client
}

func (s *clientCreator) CreateClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (client.Client, error) {
	kubeConfigBytes, err := s.getKubeConfigBytesFromSecret(ctx, logger)
	if err != nil {
		return nil, err
	}
	return util.CreateClientFromKubeConfigBytes(kubeConfigBytes, connectionTimeout)
}

func (s *clientCreator) CreateDiscoveryClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (discovery.DiscoveryInterface, error) {
	kubeConfigBytes, err := s.getKubeConfigBytesFromSecret(ctx, logger)
	if err != nil {
		return nil, err
	}
	return util.CreateDiscoveryInterfaceFromKubeConfigBytes(kubeConfigBytes, connectionTimeout)
}

func (s *clientCreator) getKubeConfigBytesFromSecret(ctx context.Context, logger logr.Logger) ([]byte, error) {
	operation := fmt.Sprintf("get-secret-%s-for-namespace-%s", s.secretName, s.namespace)
	retryResult := util.Retry(ctx, logger,
		operation,
		func() ([]byte, error) {
			return util.GetKubeConfigFromSecret(ctx, s.namespace, s.secretName, s.client, logger)
		},
		defaultGetSecretMaxAttempts,
		defaultGetSecretBackoff,
		canRetrySecretGet)
	if retryResult.Err != nil {
		return nil, retryResult.Err
	}
	return retryResult.Value, nil
}

func canRetrySecretGet(err error) bool {
	return !apierrors.IsNotFound(err)
}
