package util

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gardener/dependency-watchdog/internal/test"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	secretPath     = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath = filepath.Join("testdata", "kubeconfig.yaml")
	deploymentPath = filepath.Join("testdata", "deployment.yaml")
	ctx            context.Context
	k8sClient      client.Client
	testEnv        test.ControllerTestEnv
	cfg            *rest.Config
	err            error
)

func beforeAll(t *testing.T) {
	g := NewWithT(t)
	t.Log("setting up envTest")
	test.FileExistsOrFail(secretPath)
	test.FileExistsOrFail(deploymentPath)
	test.FileExistsOrFail(kubeConfigPath)
	testEnv, err = test.CreateControllerTestEnv()
	g.Expect(err).To(BeNil())
	k8sClient = testEnv.GetClient()
	cfg = testEnv.GetConfig()
}

func TestSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"Secret not found", testSecretNotFound},
		{"Kubeconfig not found", testKubeConfigNotFound},
		{"Extract Kubeconfig from secret", testExtractKubeConfigFromSecret},
		{"Deployment not found ", testDeploymentNotFound},
		{"Deployment is found", testFoundDeployment},
		{"Timeout before getting the deployment", testTimeoutBeforeGettingDeployment},
		{"Create Scales Getter", testCreateScalesGetter},
		{"Create client from kubeconfig", testCreateClientFromKubeconfigBytes},
	}
	beforeAll(t)
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
		deleteAllDeployments(t)
	}
	t.Log("deleting envTest")
	testEnv.Delete()
}

func deleteAllDeployments(t *testing.T) {
	g := NewWithT(t)
	opts := []client.DeleteAllOfOption{client.InNamespace("default")}
	err := k8sClient.DeleteAllOf(ctx, &appsv1.Deployment{}, opts...)
	g.Expect(err).To(BeNil())
}

func testSecretNotFound(t *testing.T) {
	g := NewWithT(t)
	secret, _ := setupGetKubeconfigTest(t, k8sClient)
	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	g.Expect(kubeconfig).Should(BeNil())
}

func testKubeConfigNotFound(t *testing.T) {
	g := NewWithT(t)
	secret, cleanup := setupGetKubeconfigTest(t, k8sClient)
	defer cleanup()
	err := k8sClient.Create(ctx, secret)
	g.Expect(err).Should(BeNil())
	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(kubeconfig).Should(BeNil())
	g.Expect(err).ShouldNot(BeNil())
	g.Expect(apierrors.IsNotFound(err)).Should(BeFalse())
}

func testExtractKubeConfigFromSecret(t *testing.T) {
	g := NewWithT(t)
	secret, cleanup := setupGetKubeconfigTest(t, k8sClient)
	defer cleanup()
	kubeconfigBuffer, err := test.ReadFile(kubeConfigPath)
	g.Expect(err).Should(BeNil())
	kubeconfig := kubeconfigBuffer.Bytes()
	g.Expect(kubeconfig).ShouldNot(BeNil())

	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfig,
	}
	err = k8sClient.Create(ctx, secret)
	g.Expect(err).Should(BeNil())

	actualKubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
	g.Expect(err).Should(BeNil())
	g.Expect(actualKubeconfig).Should(Equal(kubeconfig))
}

func testDeploymentNotFound(t *testing.T) {
	timeout := 20 * time.Millisecond
	table := []struct {
		timeout *time.Duration
	}{
		{nil},
		{&timeout},
	}
	for _, entry := range table {
		g := NewWithT(t)
		deployment := setupGetDeploymentTest(t)
		actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient, entry.timeout)
		g.Expect(err).ShouldNot(BeNil())
		g.Expect(actual).Should(BeNil())
	}

}

func testFoundDeployment(t *testing.T) {
	timeout := 20 * time.Millisecond
	table := []struct {
		timeout *time.Duration
	}{
		{nil},
		{&timeout},
	}
	for _, entry := range table {
		g := NewWithT(t)
		deployment := setupGetDeploymentTest(t)

		err := k8sClient.Create(ctx, deployment)
		g.Expect(err).Should(BeNil())

		actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient, entry.timeout)
		g.Expect(err).Should(BeNil())
		g.Expect(actual).ShouldNot(BeNil())
		g.Expect(actual.ObjectMeta.Name).Should(Equal(deployment.ObjectMeta.Name))
		g.Expect(actual.ObjectMeta.Namespace).Should(Equal(deployment.ObjectMeta.Namespace))

		err = k8sClient.Delete(ctx, deployment)
		g.Expect(err).Should(BeNil())
	}

}

func testTimeoutBeforeGettingDeployment(t *testing.T) {
	g := NewWithT(t)
	deployment := setupGetDeploymentTest(t)

	err := k8sClient.Create(ctx, deployment)
	g.Expect(err).Should(BeNil())

	timeout := time.Nanosecond
	actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient, &timeout)
	g.Expect(err).ShouldNot(BeNil())
	g.Expect(err).To(Equal(context.DeadlineExceeded))
	g.Expect(actual).Should(BeNil())
}

func testCreateScalesGetter(t *testing.T) {
	g := NewWithT(t)
	scaleGetter, err := CreateScalesGetter(cfg)
	g.Expect(err).Should(BeNil())
	g.Expect(scaleGetter).ShouldNot(BeNil())
}

func testCreateClientFromKubeconfigBytes(t *testing.T) {
	g := NewWithT(t)
	kubeconfigBuffer, err := test.ReadFile(kubeConfigPath)
	g.Expect(err).Should(BeNil())
	kubeconfig := kubeconfigBuffer.Bytes()
	g.Expect(kubeconfig).ShouldNot(BeNil())

	cfg, err := CreateClientFromKubeConfigBytes(kubeconfig, time.Second)
	g.Expect(err).Should(BeNil())
	g.Expect(cfg).ShouldNot(BeNil())
}

func setupGetKubeconfigTest(t *testing.T, k8sClient client.Client) (*corev1.Secret, func()) {
	g := NewWithT(t)
	ctx = context.Background()
	secret, err := test.GetStructured[corev1.Secret](secretPath)
	g.Expect(err).Should(BeNil())
	g.Expect(secret).ShouldNot(BeNil())

	return secret, func() {
		err := k8sClient.Delete(ctx, secret)
		g.Expect(err).Should(BeNil())
	}
}

func setupGetDeploymentTest(t *testing.T) *appsv1.Deployment {
	g := NewWithT(t)
	ctx = context.Background()
	deployment, err := test.GetStructured[appsv1.Deployment](deploymentPath)
	g.Expect(err).Should(BeNil())
	g.Expect(deployment).ShouldNot(BeNil())
	return deployment
}
