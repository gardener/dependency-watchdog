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

//go:build !kind_tests

package prober

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testenv "github.com/gardener/dependency-watchdog/internal/test"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	secretPath            = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath        = filepath.Join("testdata", "kubeconfig.yaml")
	envTest               testenv.ControllerTestEnv
	sk8sClient            client.Client
	sctx                  = context.Background()
	secret                *corev1.Secret
	clientCreator         ShootClientCreator
	shootClientTestLogger = logr.Discard()
)

func TestSuite(t *testing.T) {
	var err error
	g := NewWithT(t)
	tests := []struct {
		name        string
		description string
		run         func(t *testing.T, namespace string)
	}{
		{"testSecretNotFound", "secret not found", testSecretNotFound},
		{"testConfigNotFound", "kubeconfig not found", testConfigNotFound},
		{"testCreateShootClient", "shootclient should be created", testCreateShootClient},
	}
	envTest, err = testenv.CreateDefaultControllerTestEnv(scheme.Scheme, nil)
	g.Expect(err).ToNot(HaveOccurred())
	sk8sClient = envTest.GetClient()
	for _, test := range tests {
		ns := testenv.CreateTestNamespace(context.Background(), g, sk8sClient, strings.ToLower(test.name))
		t.Run(test.description, func(t *testing.T) {
			test.run(t, ns)
		})
	}
	envTest.Delete()
}

func testSecretNotFound(t *testing.T, namespace string) {
	g := NewWithT(t)
	setupShootClientTest(t, namespace)
	k8sInterface, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(k8sInterface).ToNot(HaveOccurred())
}

func testConfigNotFound(t *testing.T, namespace string) {
	g := NewWithT(t)
	teardown := setupShootClientTest(t, namespace)
	defer teardown()
	err := sk8sClient.Create(sctx, secret)
	g.Expect(err).ToNot(HaveOccurred())
	shootClient, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(err)).To(BeFalse())
	g.Expect(shootClient).ToNot(HaveOccurred())

}

func testCreateShootClient(t *testing.T, namespace string) {
	g := NewWithT(t)
	teardown := setupShootClientTest(t, namespace)
	defer teardown()
	kubeconfig, err := testenv.ReadFile(kubeConfigPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(kubeconfig).ToNot(BeNil())
	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfig.Bytes(),
	}
	err = sk8sClient.Create(sctx, secret)
	g.Expect(err).ToNot(HaveOccurred())

	shootClient, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(shootClient).ToNot(BeNil())
}

func setupShootClientTest(t *testing.T, namespace string) func() {
	var err error
	g := NewWithT(t)
	testenv.FileExistsOrFail(secretPath)
	secret, err = testenv.GetStructured[corev1.Secret](secretPath)
	secret.ObjectMeta.Namespace = namespace
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(secret).ToNot(BeNil())
	clientCreator = NewShootClientCreator(sk8sClient)

	return func() {
		err := sk8sClient.Delete(sctx, secret)
		g.Expect(err).ShouldNot(HaveOccurred())
	}
}
