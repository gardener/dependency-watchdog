package shoot

import (
	"context"
	"github.com/gardener/dependency-watchdog/internal/prober/shoot"
	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
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

func NewFakeShootClientBuilder(discoveryClient discovery.DiscoveryInterface, client client.Client) *shootClientBuilder {
	return &shootClientBuilder{
		shootClientCreator: shootClientCreator{
			discoveryClient: discoveryClient,
			client:          client,
		},
	}
}

func (s *shootClientBuilder) WithDiscoveryClientCreationError(err error) *shootClientBuilder {
	s.shootClientCreator.discoveryClientCreationError = err
	return s
}

func (s *shootClientBuilder) WithClientCreationError(err error) *shootClientBuilder {
	s.shootClientCreator.clientCreationError = err
	return s
}

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
