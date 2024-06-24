// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"context"
	"fmt"
	"k8s.io/client-go/discovery"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewShootClientCreator creates an instance of ShootClientCreator.
func NewShootClientCreator(namespace string, secretName string, client client.Client) ShootClientCreator {
	return &shootClientCreator{
		namespace:  namespace,
		secretName: secretName,
		client:     client,
	}
}

type shootClientCreator struct {
	namespace  string
	secretName string
	client     client.Client
}

func (s *shootClientCreator) CreateClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (client.Client, error) {
	kubeConfigBytes, err := s.getKubeConfigBytesFromSecret(ctx, logger)
	if err != nil {
		return nil, err
	}
	return util.CreateClientFromKubeConfigBytes(kubeConfigBytes, connectionTimeout)
}

func (s *shootClientCreator) CreateDiscoveryClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (discovery.DiscoveryInterface, error) {
	kubeConfigBytes, err := s.getKubeConfigBytesFromSecret(ctx, logger)
	if err != nil {
		return nil, err
	}
	return util.CreateDiscoveryInterfaceFromKubeConfigBytes(kubeConfigBytes, connectionTimeout)
}

func (s *shootClientCreator) getKubeConfigBytesFromSecret(ctx context.Context, logger logr.Logger) ([]byte, error) {
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
