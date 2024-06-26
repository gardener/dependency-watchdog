// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package shoot

import (
	"context"
	"github.com/gardener/dependency-watchdog/internal/prober/fakes/k8s"
	corev1 "k8s.io/api/core/v1"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	"github.com/gardener/dependency-watchdog/internal/test"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	secretPath     = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath = filepath.Join("testdata", "kubeconfig.yaml")
	//envTest        test.ControllerTestEnv
)

func TestSuite(t *testing.T) {
	var err error
	g := NewWithT(t)
	testCases := []struct {
		name        string
		description string
		run         func(ctx context.Context, t *testing.T, namespace string, k8sClient client.Client)
	}{
		{"testSecretNotFound", "secret not found", testSecretNotFound},
		{"testConfigNotFound", "kubeconfig not found", testConfigNotFound},
		{"testCreateShootClient", "shootclient should be created", testCreateShootClient},
	}
	g.Expect(err).ToNot(HaveOccurred())
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := k8s.NewFakeClientBuilder().Build()
			testNs := test.GenerateRandomAlphanumericString(g, 4)
			test.CreateTestNamespace(ctx, g, k8sClient, testNs)
			tc.run(ctx, t, testNs, k8sClient)
		})
	}
}

func testSecretNotFound(ctx context.Context, t *testing.T, namespace string, k8sClient client.Client) {
	g := NewWithT(t)
	cc := NewClientCreator(namespace, "does-not-exist", k8sClient)
	k8sInterface, err := cc.CreateClient(ctx, logr.Discard(), time.Second)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(k8sInterface).To(BeNil())
}

func testConfigNotFound(ctx context.Context, t *testing.T, namespace string, k8sClient client.Client) {
	g := NewWithT(t)
	secretName, cleanupFn := createSecret(ctx, g, secretPath, namespace, nil, k8sClient)
	defer cleanupFn()
	cc := NewClientCreator(namespace, secretName, k8sClient)
	shootClient, err := cc.CreateClient(ctx, logr.Discard(), time.Second)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(err)).To(BeFalse())
	g.Expect(shootClient).To(BeNil())
}

func testCreateShootClient(ctx context.Context, t *testing.T, namespace string, k8sClient client.Client) {
	g := NewWithT(t)

	kubeConfig, err := test.ReadFile(kubeConfigPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(kubeConfig).ToNot(BeNil())
	secretName, cleanupFn := createSecret(ctx, g, secretPath, namespace, map[string][]byte{"kubeconfig": kubeConfig.Bytes()}, k8sClient)
	defer cleanupFn()

	cc := NewClientCreator(namespace, secretName, k8sClient)
	shootClient, err := cc.CreateClient(ctx, logr.Discard(), time.Second)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(shootClient).ToNot(BeNil())
}

func createSecret(ctx context.Context, g *WithT, path, namespace string, data map[string][]byte, k8sClient client.Client) (secretName string, cleanupFn func()) {
	test.FileExistsOrFail(path)
	secret, err := test.GetStructured[corev1.Secret](path)
	g.Expect(err).ToNot(HaveOccurred())
	secret.ObjectMeta.Namespace = namespace
	secret.Data = data
	g.Expect(secret).ToNot(BeNil())
	// create the secret
	g.Expect(k8sClient.Create(ctx, secret)).ToNot(HaveOccurred())

	secretName = secret.Name
	cleanupFn = func() {
		g.Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, secret))).To(BeNil())
	}
	return
}
