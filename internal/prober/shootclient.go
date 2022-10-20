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
	"fmt"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShootClientCreator interface {
	CreateClient(ctx context.Context, namespace string, secretName string, connectionTimeout time.Duration) (kubernetes.Interface, error)
}

func NewShootClientCreator(client client.Client) ShootClientCreator {
	return &shootclientCreator{client}
}

type shootclientCreator struct {
	client.Client
}

func (s *shootclientCreator) CreateClient(ctx context.Context, namespace string, secretName string, connectionTimeout time.Duration) (kubernetes.Interface, error) {
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
	return util.CreateClientFromKubeConfigBytes(retryResult.Value, connectionTimeout)
}

func canRetrySecretGet(err error) bool {
	return !apierrors.IsNotFound(err)
}
