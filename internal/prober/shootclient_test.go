package prober

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gardener/dependency-watchdog/internal/test"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	secretPath     = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath = filepath.Join("testdata", "kubeconfig.yaml")
	envTest        test.ControllerTestEnv
	sk8sClient     client.Client
	sctx           = context.Background()
	secret         *corev1.Secret
	clientCreator  ShootClientCreator
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
	envTest, err = test.CreateControllerTestEnv()
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
	setupShootCLientTest(t)
	k8sInterface, err := clientCreator.CreateClient(sctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(k8sInterface).To(BeNil())
}

func testConfigNotFound(t *testing.T) {
	g := NewWithT(t)
	teardown := setupShootCLientTest(t)
	defer teardown()
	err := sk8sClient.Create(sctx, secret)
	g.Expect(err).To(BeNil())
	shootClient, err := clientCreator.CreateClient(sctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name)
	g.Expect(err).ToNot(BeNil())
	g.Expect(apierrors.IsNotFound(err)).To(BeFalse())
	g.Expect(shootClient).To(BeNil())

}

func testCreateShootClient(t *testing.T) {
	g := NewWithT(t)
	teardown := setupShootCLientTest(t)
	defer teardown()
	kubeconfig, err := test.ReadFile(kubeConfigPath)
	g.Expect(err).To(BeNil())
	g.Expect(kubeconfig).ToNot(BeNil())
	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfig.Bytes(),
	}
	err = sk8sClient.Create(sctx, secret)
	g.Expect(err).To(BeNil())

	shootClient, err := clientCreator.CreateClient(sctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name)
	g.Expect(err).To(BeNil())
	g.Expect(shootClient).ToNot(BeNil())
}

func setupShootCLientTest(t *testing.T) func() {
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
