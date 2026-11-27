// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package util

import (
	"testing"

	. "github.com/onsi/gomega"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestValidateKubeConfig(t *testing.T) {
	t.Parallel()

	validUser := "test-user"

	testCases := []struct {
		name        string
		authInfo    clientcmdapi.AuthInfo
		expectError bool
		errContains string
	}{
		{
			name:        "valid inline token",
			authInfo:    clientcmdapi.AuthInfo{Token: "my-token"},
			expectError: false,
		},
		{
			name:        "valid inline client certificate data",
			authInfo:    clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert"), ClientKeyData: []byte("key")},
			expectError: false,
		},
		{
			name:        "valid username and password",
			authInfo:    clientcmdapi.AuthInfo{Username: "user", Password: "pass"},
			expectError: false,
		},
		{
			name:        "rejects client-certificate file path",
			authInfo:    clientcmdapi.AuthInfo{ClientCertificate: "/etc/certs/client.crt"},
			expectError: true,
			errContains: "client-certificate",
		},
		{
			name:        "rejects client-key file path",
			authInfo:    clientcmdapi.AuthInfo{ClientKey: "/etc/certs/client.key"},
			expectError: true,
			errContains: "client-key",
		},
		{
			name:        "rejects tokenFile",
			authInfo:    clientcmdapi.AuthInfo{TokenFile: "/var/run/secrets/token"},
			expectError: true,
			errContains: "tokenFile",
		},
		{
			name:        "rejects impersonate",
			authInfo:    clientcmdapi.AuthInfo{Impersonate: "other-user"},
			expectError: true,
			errContains: "impersonation",
		},
		{
			name:        "rejects impersonate-groups",
			authInfo:    clientcmdapi.AuthInfo{ImpersonateGroups: []string{"group1"}},
			expectError: true,
			errContains: "impersonation",
		},
		{
			name:        "rejects auth-provider",
			authInfo:    clientcmdapi.AuthInfo{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "oidc"}},
			expectError: true,
			errContains: "auth-provider",
		},
		{
			name:        "rejects exec plugin",
			authInfo:    clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "aws"}},
			expectError: true,
			errContains: "exec",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			config := clientcmdapi.Config{
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					validUser: &tc.authInfo,
				},
			}
			err := validateKubeConfig(config)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.errContains))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateKubeConfigEmptyAuthInfos(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	config := clientcmdapi.Config{AuthInfos: map[string]*clientcmdapi.AuthInfo{}}
	g.Expect(validateKubeConfig(config)).ToNot(HaveOccurred())
}
