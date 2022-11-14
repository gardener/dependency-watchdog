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

package prober

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gardener/dependency-watchdog/internal/test"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	secretPath            = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath        = filepath.Join("testdata", "kubeconfig.yaml")
	envTest               test.ControllerTestEnv
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
		title string
		run   func(t *testing.T)
	}{
		{"secret not found", testSecretNotFound},
		{"kubeconfig not found", testConfigNotFound},
		{"shootclient should be created", testCreateShootClient},
	}
	envTest, err = test.CreateDefaultControllerTestEnv()
	g.Expect(err).To(BeNil())
	sk8sClient = envTest.GetClient()
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
	envTest.Delete()
}

func testSecretNotFound(t *testing.T) {
	g := NewWithT(t)
	setupShootClientTest(t)
	k8sInterface, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(k8sInterface).To(BeNil())
}

func testConfigNotFound(t *testing.T) {
	g := NewWithT(t)
	teardown := setupShootClientTest(t)
	defer teardown()
	err := sk8sClient.Create(sctx, secret)
	g.Expect(err).To(BeNil())
	shootClient, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(err).ToNot(BeNil())
	g.Expect(apierrors.IsNotFound(err)).To(BeFalse())
	g.Expect(shootClient).To(BeNil())

}

func testCreateShootClient(t *testing.T) {
	g := NewWithT(t)
	teardown := setupShootClientTest(t)
	defer teardown()
	kubeconfig, err := test.ReadFile(kubeConfigPath)
	g.Expect(err).To(BeNil())
	g.Expect(kubeconfig).ToNot(BeNil())
	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfig.Bytes(),
	}
	err = sk8sClient.Create(sctx, secret)
	g.Expect(err).To(BeNil())

	shootClient, err := clientCreator.CreateClient(sctx, shootClientTestLogger, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, time.Second)
	g.Expect(err).To(BeNil())
	g.Expect(shootClient).ToNot(BeNil())
}

func setupShootClientTest(t *testing.T) func() {
	var err error
	g := NewWithT(t)
	test.FileExistsOrFail(secretPath)
	secret, err = test.GetStructured[corev1.Secret](secretPath)
	g.Expect(err).To(BeNil())
	g.Expect(secret).ToNot(BeNil())
	clientCreator = NewShootClientCreator(sk8sClient)

	return func() {
		err := sk8sClient.Delete(sctx, secret)
		g.Expect(err).Should(BeNil())
	}
}
