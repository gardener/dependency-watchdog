// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShootClientCreator provides a facade to create kubernetes client targeting a shoot.
type ShootClientCreator interface {
	// CreateClient creates a new clientSet to connect to the Kube ApiServer running in the passed-in shoot control namespace.
	CreateClient(ctx context.Context, logger logr.Logger, namespace string, secretName string, connectionTimeout time.Duration) (kubernetes.Interface, error)
}

// NewShootClientCreator creates an instance of ShootClientCreator.
func NewShootClientCreator(client client.Client) ShootClientCreator {
	return &shootClientCreator{client}
}

type shootClientCreator struct {
	client.Client
}

func (s *shootClientCreator) CreateClient(ctx context.Context, logger logr.Logger, namespace string, secretName string, connectionTimeout time.Duration) (kubernetes.Interface, error) {
	operation := fmt.Sprintf("get-secret-%s-for-namespace-%s", secretName, namespace)
	retryResult := util.Retry(ctx, logger,
		operation,
		func() ([]byte, error) {
			return util.GetKubeConfigFromSecret(ctx, namespace, secretName, s.Client, logger)
		},
		defaultGetSecretMaxAttempts,
		defaultGetSecretBackoff,
		canRetrySecretGet)
	if retryResult.Err != nil {
		return nil, retryResult.Err
	}
	return util.CreateClientFromKubeConfigBytes(retryResult.Value, connectionTimeout)
}

func canRetrySecretGet(err error) bool {
	return !apierrors.IsNotFound(err)
}
