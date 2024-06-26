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
	discoveryClient discovery.DiscoveryInterface
	client          client.Client
}

// NewFakeShootClientCreator creates an instance of ClientCreator.
func NewFakeShootClientCreator(discoveryClient discovery.DiscoveryInterface, client client.Client) shoot.ClientCreator {
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
