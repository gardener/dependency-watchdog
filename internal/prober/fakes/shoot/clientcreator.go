// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package shoot

import (
	"context"
	"time"

	"github.com/gardener/dependency-watchdog/internal/prober/shoot"
	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type shootClientCreator struct {
	discoveryClient              discovery.DiscoveryInterface
	client                       client.Client
	discoveryClientCreationError error
	clientCreationError          error
}

type shootClientBuilder struct {
	shootClientCreator shootClientCreator
}

// NewFakeShootClientBuilder creates a new instance of shootClientBuilder.
func NewFakeShootClientBuilder(discoveryClient discovery.DiscoveryInterface, client client.Client) *shootClientBuilder {
	return &shootClientBuilder{
		shootClientCreator: shootClientCreator{
			discoveryClient: discoveryClient,
			client:          client,
		},
	}
}

// WithDiscoveryClientCreationError sets the error to be returned when creating a discovery client.
func (s *shootClientBuilder) WithDiscoveryClientCreationError(err error) *shootClientBuilder {
	s.shootClientCreator.discoveryClientCreationError = err
	return s
}

// WithClientCreationError sets the error to be returned when creating a client.
func (s *shootClientBuilder) WithClientCreationError(err error) *shootClientBuilder {
	s.shootClientCreator.clientCreationError = err
	return s
}

// Build returns a shootClientCreator.
func (s *shootClientBuilder) Build() shoot.ClientCreator {
	return &s.shootClientCreator
}

//--------------------------- Implementation of ShootClientCreator interface ---------------------------

func (s *shootClientCreator) CreateClient(_ context.Context, _ logr.Logger, _ time.Duration) (client.Client, error) {
	if s.clientCreationError != nil {
		return nil, s.clientCreationError
	}
	return s.client, nil
}

func (s *shootClientCreator) CreateDiscoveryClient(_ context.Context, _ logr.Logger, _ time.Duration) (discovery.DiscoveryInterface, error) {
	if s.discoveryClientCreationError != nil {
		return nil, s.discoveryClientCreationError
	}
	return s.discoveryClient, nil
}
