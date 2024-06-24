package fakes

import (
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/fake"
)

// discoveryClient is a test implementation of DiscoveryInterface.
type discoveryClient struct {
	discovery.DiscoveryInterface
	err error
}

func (t *discoveryClient) ServerVersion() (*version.Info, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.DiscoveryInterface.ServerVersion()
}

// NewDiscoveryClient creates a new DiscoveryClient.
func NewDiscoveryClient(err error) discovery.DiscoveryInterface {
	return &discoveryClient{
		DiscoveryInterface: fake.NewSimpleClientset().Discovery(),
		err:                err,
	}
}
