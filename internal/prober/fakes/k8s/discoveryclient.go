// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package k8s

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

// ServerVersion is the implementation of the DiscoveryInterface method for discoveryClient
func (t *discoveryClient) ServerVersion() (*version.Info, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.DiscoveryInterface.ServerVersion()
}

// NewFakeDiscoveryClient creates a new DiscoveryClient.
func NewFakeDiscoveryClient(err error) discovery.DiscoveryInterface {
	return &discoveryClient{
		DiscoveryInterface: fake.NewSimpleClientset().Discovery(),
		err:                err,
	}
}
