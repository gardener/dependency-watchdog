// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package util

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testutil "github.com/gardener/dependency-watchdog/internal/test"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kindTestClusterName = "k8s-helper-test"
)

var (
	secretPath     = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath = filepath.Join("testdata", "kubeconfig.yaml")
	deploymentPath = filepath.Join("testdata", "deployment.yaml")
	k8sClient      client.Client
	kindCluster    testutil.KindCluster
	restConfig     *rest.Config
	testScaler     scale.ScaleInterface
	testLogger     = log.Log.WithName(kindTestClusterName)
)

type testCleanup func(*WithT)

func beforeAll(t *testing.T) {
	var err error
	g := NewWithT(t)
	t.Log("setting up kind cluster", "name:", kindTestClusterName)
	testutil.FileExistsOrFail(secretPath)
	testutil.FileExistsOrFail(deploymentPath)
	testutil.FileExistsOrFail(kubeConfigPath)
	kindCluster, err = testutil.CreateKindCluster(testutil.KindConfig{Name: "k8s-helper-test"})
	g.Expect(err).To(BeNil())
	k8sClient = kindCluster.GetClient()
	restConfig = kindCluster.GetRestConfig()
}

func afterAll(t *testing.T) {
	g := NewWithT(t)
	t.Log("deleting kind cluster", "name:", kindTestClusterName)
	err := kindCluster.Delete()
	g.Expect(err).To(BeNil())
}

func TestSuitForK8sHelper(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"secret not found", testSecretNotFound},
		{"extract KubeConfig from secret", testExtractKubeConfigFromSecret},
		{"secret with no KubeConfig", testExtractKubeConfigFromSecretWithNoKubeConfig},
		{"create client from KubeConfig", testCreateClientFromKubeConfigBytes},
		{"create transport with keep-alive disabled", testCreateTransportWithDisabledKeepAlive},
		{"create scales getter", testCreateScalesGetter},
		{"get scale resource", testGetScaleResource},
	}

	beforeAll(t)
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
	afterAll(t)
}

func testSecretNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	sec, cleanup := getSecretFromFile(ctx, g)
	defer cleanup(g)
	kubeConfig, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	g.Expect(kubeConfig).Should(BeNil())
}

func testExtractKubeConfigFromSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	sec, cleanup := getSecretFromFile(ctx, g)
	defer cleanup(g)
	kubeConfigBytes := createKubeConfigSecret(ctx, g, sec, &kubeConfigPath)

	// test and assertions
	actualKubeConfigBytes, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient)
	g.Expect(err).Should(BeNil())
	g.Expect(actualKubeConfigBytes).Should(Equal(kubeConfigBytes))
}

func testExtractKubeConfigFromSecretWithNoKubeConfig(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	sec, cleanup := getSecretFromFile(ctx, g)
	defer cleanup(g)
	createKubeConfigSecret(ctx, g, sec, nil)

	// test and assertions
	_, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient)
	g.Expect(err).ToNot(BeNil())
}

func testCreateClientFromKubeConfigBytes(t *testing.T) {
	g := NewWithT(t)
	kubeConfigBytes := getKubeConfigBytes(g, kubeConfigPath)

	cfg, err := CreateClientFromKubeConfigBytes(kubeConfigBytes, time.Second)
	g.Expect(err).Should(BeNil())
	g.Expect(cfg).ShouldNot(BeNil())
}

func testCreateTransportWithDisabledKeepAlive(t *testing.T) {
	g := NewWithT(t)
	config := getRestConfig(g, kubeConfigPath)

	transport, err := createTransportWithDisabledKeepAlive(config)
	g.Expect(err).Should(BeNil())
	g.Expect(transport.DisableKeepAlives).To(Equal(true))
}

func testCreateScalesGetter(t *testing.T) {
	g := NewWithT(t)
	config := getRestConfig(g, kubeConfigPath)

	scalesGetter, err := CreateScalesGetter(config)
	g.Expect(err).To(BeNil())
	g.Expect(scalesGetter).ToNot(BeNil())
}

func testGetScaleResource(t *testing.T) {
	const (
		resourceGroup = "apps"
	)
	g := NewWithT(t)
	ctx := context.Background()
	config := getRestConfig(g, kubeConfigPath)
	scalesGetter, err := CreateScalesGetter(config)
	g.Expect(err).To(BeNil())
	g.Expect(scalesGetter).ToNot(BeNil())

	deployment, cleanup := createDeployment(ctx, g, deploymentPath)
	defer cleanup(g)

	resourceRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       deployment.Kind,
		Name:       deployment.Name,
		APIVersion: deployment.APIVersion,
	}

	scaler := scalesGetter.Scales(deployment.Namespace)
	groupResource, scaleRes, err := GetScaleResource(ctx, k8sClient, scaler, logger, resourceRef, 20*time.Second)
	g.Expect(err).To(BeNil())
	g.Expect(groupResource.Group).To(Equal(resourceGroup))
	g.Expect(groupResource.Resource).To(Equal(deployment.Kind))
	g.Expect(scaleRes.Name).To(Equal(deployment.Name))
	g.Expect(scaleRes.Namespace).To(Equal(deployment.Namespace))
}

//func setupGetDeploymentTest(t *testing.T) *appsv1.Deployment {
//	g := NewWithT(t)
//	ctx = context.Background()
//	deployment, err := test.GetStructured[appsv1.Deployment](deploymentPath)
//	g.Expect(err).Should(BeNil())
//	g.Expect(deployment).ShouldNot(BeNil())
//	return deployment
//}

func getSecretFromFile(ctx context.Context, g *WithT) (*corev1.Secret, testCleanup) {
	secret, err := testutil.GetStructured[corev1.Secret](secretPath)
	g.Expect(err).To(BeNil())
	g.Expect(secret).ShouldNot(BeNil())
	return secret, func(g *WithT) {
		err := client.IgnoreNotFound(k8sClient.Delete(ctx, secret))
		g.Expect(err).Should(BeNil())
	}
}

func createKubeConfigSecret(ctx context.Context, g *WithT, sec *corev1.Secret, kubeConfigPath *string) []byte {
	var kubeConfigBytes []byte
	if kubeConfigPath != nil {
		kubeConfigBytes = getKubeConfigBytes(g, *kubeConfigPath)
		sec.Data = map[string][]byte{
			kubeConfigSecretKey: kubeConfigBytes,
		}
	}
	err := k8sClient.Create(ctx, sec)
	g.Expect(err).Should(BeNil())
	return kubeConfigBytes
}

func createDeployment(ctx context.Context, g *WithT, yamlPath string) (*appsv1.Deployment, testCleanup) {
	d, cleanup := getDeploymentFromFile(ctx, g, yamlPath)
	err := k8sClient.Create(ctx, d)
	g.Expect(err).To(BeNil())
	// post create TypeMeta is blanked out @see https://github.com/kubernetes-sigs/controller-runtime/issues/1517
	gvks, unversioned, err := k8sClient.Scheme().ObjectKinds(d)
	g.Expect(err).To(BeNil())
	if !unversioned && len(gvks) == 1 {
		d.SetGroupVersionKind(gvks[0])
	}
	return d, cleanup
}

func getDeploymentFromFile(ctx context.Context, g *WithT, yamlPath string) (*appsv1.Deployment, testCleanup) {
	d, err := testutil.GetStructured[appsv1.Deployment](yamlPath)
	g.Expect(err).To(BeNil())
	g.Expect(d).ShouldNot(BeNil())
	return d, func(g *WithT) {
		err := client.IgnoreNotFound(k8sClient.Delete(ctx, d))
		g.Expect(err).To(BeNil())
	}
}

func getKubeConfigBytes(g *WithT, path string) []byte {
	kubeConfigBuf, err := testutil.ReadFile(path)
	g.Expect(err).Should(BeNil())
	kubeConfigBytes := kubeConfigBuf.Bytes()
	g.Expect(kubeConfigBytes).ShouldNot(BeNil())
	return kubeConfigBytes
}

func getRestConfig(g *WithT, kubeConfigPath string) *rest.Config {
	kubeConfigBytes := getKubeConfigBytes(g, kubeConfigPath)
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	g.Expect(err).Should(BeNil())
	config, err := clientConfig.ClientConfig()
	g.Expect(err).Should(BeNil())
	return config
}

//func TestSuite(t *testing.T) {
//	tests := []struct {
//		title string
//		run   func(t *testing.T)
//	}{
//		{"Secret not found", testSecretNotFound},
//		{"Kubeconfig not found", testKubeConfigNotFound},
//		{"Extract Kubeconfig from secret", testExtractKubeConfigFromSecret},
//		{"Deployment not found ", testDeploymentNotFound},
//		{"Deployment is found", testFoundDeployment},
//		{"Timeout before getting the deployment", testTimeoutBeforeGettingDeployment},
//		{"Create Scales Getter", testCreateScalesGetter},
//		{"Create client from kubeconfig", testCreateClientFromKubeconfigBytes},
//		{"KeepAlive is disabled", testCreateTransportWithDisabledKeepAlive},
//	}
//	beforeAll(t)
//	// NOTE: when the tests(second and later) are run individually using goland tool,
//	// then they fail as goland cannot pinpoint the exact test when using a for loop to run the tests
//	for _, test := range tests {
//		t.Run(test.title, func(t *testing.T) {
//			test.run(t)
//		})
//		deleteAllDeployments(t)
//	}
//	t.Log("deleting envTest")
//	testEnv.Delete()
//}
//
//func deleteAllDeployments(t *testing.T) {
//	g := NewWithT(t)
//	opts := []client.DeleteAllOfOption{client.InNamespace("default")}
//	err := k8sClient.DeleteAllOf(ctx, &appsv1.Deployment{}, opts...)
//	g.Expect(err).To(BeNil())
//}
//
//func testSecretNotFound(t *testing.T) {
//	g := NewWithT(t)
//	secret, _ := setupGetKubeconfigTest(t, k8sClient)
//	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
//	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
//	g.Expect(kubeconfig).Should(BeNil())
//}
//
//func testKubeConfigNotFound(t *testing.T) {
//	g := NewWithT(t)
//	secret, testCleanup := setupGetKubeconfigTest(t, k8sClient)
//	defer testCleanup()
//	err := k8sClient.Create(ctx, secret)
//	g.Expect(err).Should(BeNil())
//	kubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
//	g.Expect(kubeconfig).Should(BeNil())
//	g.Expect(err).ShouldNot(BeNil())
//	g.Expect(apierrors.IsNotFound(err)).Should(BeFalse())
//}
//
//func testExtractKubeConfigFromSecret(t *testing.T) {
//	g := NewWithT(t)
//	secret, testCleanup := setupGetKubeconfigTest(t, k8sClient)
//	defer testCleanup()
//	kubeconfigBuffer, err := test.ReadFile(kubeConfigPath)
//	g.Expect(err).Should(BeNil())
//	kubeconfig := kubeconfigBuffer.Bytes()
//	g.Expect(kubeconfig).ShouldNot(BeNil())
//
//	secret.Data = map[string][]byte{
//		"kubeconfig": kubeconfig,
//	}
//	err = k8sClient.Create(ctx, secret)
//	g.Expect(err).Should(BeNil())
//
//	actualKubeconfig, err := GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
//	g.Expect(err).Should(BeNil())
//	g.Expect(actualKubeconfig).Should(Equal(kubeconfig))
//}
//
//func testDeploymentNotFound(t *testing.T) {
//	timeout := 20 * time.Millisecond
//	table := []struct {
//		timeout *time.Duration
//	}{
//		{nil},
//		{&timeout},
//	}
//	for _, entry := range table {
//		g := NewWithT(t)
//		deployment := setupGetDeploymentTest(t)
//		actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient, entry.timeout)
//		g.Expect(err).ShouldNot(BeNil())
//		g.Expect(actual).Should(BeNil())
//	}
//
//}
//
//func testFoundDeployment(t *testing.T) {
//	timeout := 20 * time.Millisecond
//	table := []struct {
//		timeout *time.Duration
//	}{
//		{nil},
//		{&timeout},
//	}
//	for _, entry := range table {
//		g := NewWithT(t)
//		deployment := setupGetDeploymentTest(t)
//
//		err := k8sClient.Create(ctx, deployment)
//		g.Expect(err).Should(BeNil())
//
//		actual, err := GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient, entry.timeout)
//		g.Expect(err).Should(BeNil())
//		g.Expect(actual).ShouldNot(BeNil())
//		g.Expect(actual.ObjectMeta.Name).Should(Equal(deployment.ObjectMeta.Name))
//		g.Expect(actual.ObjectMeta.Namespace).Should(Equal(deployment.ObjectMeta.Namespace))
//
//		err = k8sClient.Delete(ctx, deployment)
//		g.Expect(err).Should(BeNil())
//	}
//
//}
//
//func testTimeoutBeforeGettingDeployment(t *testing.T) {
//	g := NewWithT(t)
//	deployment := setupGetDeploymentTest(t)
//
//	err := k8sClient.Create(ctx, deployment)
//	g.Expect(err).Should(BeNil())
//
//	timeout := time.Nanosecond
//	actual, err := GetScaleResource(ctx, k8sClient, , testLogger, ,&timeout)
//	g.Expect(err).ShouldNot(BeNil())
//	g.Expect(err.Error()).Should(ContainSubstring(context.DeadlineExceeded.Error()))
//	g.Expect(actual).Should(BeNil())
//}
//
//func testCreateScalesGetter(t *testing.T) {
//	g := NewWithT(t)
//	scaleGetter, err := CreateScalesGetter(cfg)
//	g.Expect(err).Should(BeNil())
//	g.Expect(scaleGetter).ShouldNot(BeNil())
//}
//
//func testCreateClientFromKubeconfigBytes(t *testing.T) {
//	g := NewWithT(t)
//	kubeconfigBuffer, err := test.ReadFile(kubeConfigPath)
//	g.Expect(err).Should(BeNil())
//	kubeconfig := kubeconfigBuffer.Bytes()
//	g.Expect(kubeconfig).ShouldNot(BeNil())
//
//	cfg, err := CreateClientFromKubeConfigBytes(kubeconfig, time.Second)
//	g.Expect(err).Should(BeNil())
//	g.Expect(cfg).ShouldNot(BeNil())
//}
//
//func testCreateTransportWithDisabledKeepAlive(t *testing.T) {
//	//Setup
//	g := NewWithT(t)
//	kubeconfigBuffer, err := test.ReadFile(kubeConfigPath)
//	g.Expect(err).Should(BeNil())
//	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigBuffer.Bytes())
//	g.Expect(err).Should(BeNil())
//	config, err := clientConfig.ClientConfig()
//	g.Expect(err).Should(BeNil())
//
//	//Test
//	transport, err := createTransportWithDisabledKeepAlive(config)
//	g.Expect(err).Should(BeNil())
//	g.Expect(transport.DisableKeepAlives).To(Equal(true))
//}
//
//func setupGetKubeconfigTest(t *testing.T, k8sClient client.Client) (*corev1.Secret, func()) {
//	g := NewWithT(t)
//	ctx = context.Background()
//	secret, err := test.GetStructured[corev1.Secret](secretPath)
//	g.Expect(err).Should(BeNil())
//	g.Expect(secret).ShouldNot(BeNil())
//
//	return secret, func() {
//		err := k8sClient.Delete(ctx, secret)
//		g.Expect(err).Should(BeNil())
//	}
//}
//
//func setupGetDeploymentTest(t *testing.T) *appsv1.Deployment {
//	g := NewWithT(t)
//	ctx = context.Background()
//	deployment, err := test.GetStructured[appsv1.Deployment](deploymentPath)
//	g.Expect(err).Should(BeNil())
//	g.Expect(deployment).ShouldNot(BeNil())
//	return deployment
//}
