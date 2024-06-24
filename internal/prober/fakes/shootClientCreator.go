package fakes

import (
	"context"
	"github.com/gardener/dependency-watchdog/internal/prober"
	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type shootClientCreator struct {
	discoveryClient discovery.DiscoveryInterface
	client          client.Client
}

// NewShootClientCreator creates an instance of ShootClientCreator.
func NewShootClientCreator(discoveryClient discovery.DiscoveryInterface, client client.Client) prober.ShootClientCreator {
	return &shootClientCreator{
		discoveryClient: discoveryClient,
		client:          client,
	}
}

func (s *shootClientCreator) CreateClient(_ context.Context, _ logr.Logger, _ time.Duration) (client.Client, error) {
	return s.client, nil
}

func (s *shootClientCreator) CreateDiscoveryClient(_ context.Context, _ logr.Logger, _ time.Duration) (discovery.DiscoveryInterface, error) {
	return s.discoveryClient, nil
}
