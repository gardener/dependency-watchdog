package types

import (
	"context"
	papi "github.com/gardener/dependency-watchdog/api/prober"
	dwdScaler "github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// Prober represents a probe to the Kube ApiServer of a shoot
type Prober struct {
	Namespace            string
	Config               *papi.Config
	WorkerNodeConditions map[string][]string
	Scaler               dwdScaler.Scaler
	SeedClient           client.Client
	ShootClientCreator   ShootClientCreator
	BackOff              *time.Timer
	Ctx                  context.Context
	CancelFn             context.CancelFunc
	Logger               logr.Logger
}

// Manager is the convenience interface to manage lifecycle of probers.
type Manager interface {
	// Register registers the given prober with the manager. It should return false if prober is already registered.
	Register(prober Prober) bool
	// Unregister closes the prober and removes it from the manager. It should return false if prober is not registered with the manager.
	Unregister(key string) bool
	// GetProber uses the given key to get a registered prober from the manager. It returns false if prober is not found.
	GetProber(key string) (Prober, bool)
	// GetAllProbers returns a slice of all the probers registered with the manager.
	GetAllProbers() []Prober
}

// ShootClientCreator provides a facade to create kubernetes client targeting a shoot.
type ShootClientCreator interface {
	// CreateClient creates a new client.Client to connect to the Kube ApiServer running in the passed-in shoot control namespace.
	CreateClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (client.Client, error)
	// CreateDiscoveryClient creates a new discovery.DiscoveryInterface to connect to the Kube ApiServer running in the passed-in shoot control namespace.
	CreateDiscoveryClient(ctx context.Context, logger logr.Logger, connectionTimeout time.Duration) (discovery.DiscoveryInterface, error)
}
