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
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testutil "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kindTestClusterName = "k8s-helper-test"
)

var (
	secretPath          = filepath.Join("testdata", "secret.yaml")
	kubeConfigPath      = filepath.Join("testdata", "kubeconfig.yaml")
	deploymentPath      = filepath.Join("testdata", "deployment.yaml")
	k8sClient           client.Client
	kindCluster         testutil.KindCluster
	restConfig          *rest.Config
	k8sHelperTestLogger = logr.Discard()
)

type testCleanup func(*WithT)

func beforeAll(t *testing.T) {
	var err error
	g := NewWithT(t)
	testutil.FileExistsOrFail(secretPath)
	testutil.FileExistsOrFail(deploymentPath)
	testutil.FileExistsOrFail(kubeConfigPath)

	t.Log("setting up kind cluster", "name:", kindTestClusterName)
	kindCluster, err = testutil.CreateKindCluster(testutil.KindConfig{Name: kindTestClusterName})
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
		{"get scale resource for not supported GVK", testGetScaleResourceForUnsupportedGKV},
		{"get ready replicas for a resource that does not exist", testGetReadyReplicasForNonExistingResource},
		{"get ready replicas for a resource with zero spec.replicas", testGetReadyReplicasForResourceWithZeroReplicas},
		{"get ready replicas for a resource with spec.replicas greater than zero", testGetReadyReplicasForResourceWithNonZeroReplicas},
		{"get resource annotations", testGetResourceAnnotations},
		{"get resource annotations with no annotations set", testGetResourceAnnotationsWhenNoneExists},
		{"patch resource annotations", testPatchResourceAnnotations},
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
	kubeConfig, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient, k8sHelperTestLogger)
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
	actualKubeConfigBytes, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient, k8sHelperTestLogger)
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
	_, err := GetKubeConfigFromSecret(ctx, sec.ObjectMeta.Namespace, sec.ObjectMeta.Name, k8sClient, k8sHelperTestLogger)
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
		resourceGroup      = "apps"
		deploymentResource = "deployments"
	)
	g := NewWithT(t)
	ctx := context.Background()
	scalesGetter, err := CreateScalesGetter(restConfig)
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
	groupResource, scaleRes, err := GetScaleResource(ctx, k8sClient, scaler, k8sHelperTestLogger, resourceRef, 20*time.Second)
	g.Expect(err).To(BeNil())

	g.Expect(groupResource.Group).To(Equal(resourceGroup))
	g.Expect(groupResource.Resource).To(Equal(deploymentResource))
	g.Expect(scaleRes.Name).To(Equal(deployment.Name))
	g.Expect(scaleRes.Namespace).To(Equal(deployment.Namespace))
}

func testGetScaleResourceForUnsupportedGKV(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	scalesGetter, err := CreateScalesGetter(restConfig)
	g.Expect(err).To(BeNil())
	g.Expect(scalesGetter).ToNot(BeNil())

	resourceRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       "Machine",
		Name:       "m1",
		APIVersion: "v1alpha1",
	}

	scaler := scalesGetter.Scales("default")
	_, _, err = GetScaleResource(ctx, k8sClient, scaler, k8sHelperTestLogger, resourceRef, 20*time.Second)
	g.Expect(err).ToNot(BeNil())
}

func testGetReadyReplicasForNonExistingResource(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"
	resourceRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       "Deployment",
		Name:       "DoesNotExist",
		APIVersion: "apps/v1",
	}

	_, err := GetResourceReadyReplicas(ctx, k8sClient, namespace, resourceRef)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func testGetReadyReplicasForResourceWithZeroReplicas(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"
	// create a deployment with zero replicas. Status will not have any ready replicas in this case
	deployment, cleanup := createDeployment(ctx, g, deploymentPath)
	defer cleanup(g)
	resourceRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       deployment.Kind,
		Name:       deployment.Name,
		APIVersion: deployment.APIVersion,
	}

	readyReplicas, err := GetResourceReadyReplicas(ctx, k8sClient, namespace, resourceRef)
	g.Expect(err).To(BeNil())
	g.Expect(readyReplicas).To(Equal(int32(0)))
}

func testGetReadyReplicasForResourceWithNonZeroReplicas(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"
	var (
		err      error
		replicas int32
	)
	d, cleanup := getDeploymentFromFile(ctx, g, deploymentPath)
	defer cleanup(g)
	d.Spec.Replicas = pointer.Int32(1)
	err = k8sClient.Create(ctx, d)
	g.Expect(err).To(BeNil())

	g.Eventually(func() int32 {
		replicas, err = GetResourceReadyReplicas(ctx, k8sClient, namespace, &autoscalingv1.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       d.Name,
			APIVersion: "apps/v1",
		})
		g.Expect(err).To(BeNil())
		return replicas
	}, "30s").Should(Equal(int32(1)))
}

func testGetResourceAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"
	var err error
	d, cleanup := getDeploymentFromFile(ctx, g, deploymentPath)
	defer cleanup(g)
	metav1.SetMetaDataAnnotation(&d.ObjectMeta, "test.gardener.cloud/bingo", "tringo")
	err = k8sClient.Create(ctx, d)
	g.Expect(err).To(BeNil())

	annotations, err := GetResourceAnnotations(ctx, k8sClient, namespace, &autoscalingv1.CrossVersionObjectReference{
		Kind:       "Deployment",
		Name:       d.Name,
		APIVersion: "apps/v1",
	})
	g.Expect(err).To(BeNil())
	g.Expect(annotations).Should(HaveKeyWithValue("test.gardener.cloud/bingo", "tringo"))
}

func testGetResourceAnnotationsWhenNoneExists(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"
	var err error
	d, cleanup := getDeploymentFromFile(ctx, g, deploymentPath)
	defer cleanup(g)
	err = k8sClient.Create(ctx, d)
	g.Expect(err).To(BeNil())

	annotations, err := GetResourceAnnotations(ctx, k8sClient, namespace, &autoscalingv1.CrossVersionObjectReference{
		Kind:       "Deployment",
		Name:       d.Name,
		APIVersion: "apps/v1",
	})
	g.Expect(err).To(BeNil())
	g.Expect(annotations).To(BeNil())
}

func testPatchResourceAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	const namespace = "default"

	// create a deployment with zero replicas. Status will not have any ready replicas in this case
	deployment, cleanup := createDeployment(ctx, g, deploymentPath)
	defer cleanup(g)
	resourceRef := &autoscalingv1.CrossVersionObjectReference{
		Kind:       deployment.Kind,
		Name:       deployment.Name,
		APIVersion: deployment.APIVersion,
	}
	expectedAnnotations := map[string]string{
		"test.gardener.cloud/ignored":  "true",
		"test.gardener.cloud/replicas": "2",
	}

	patchBytes := createAnnotationPatchBytes(g, expectedAnnotations)
	err := PatchResourceAnnotations(ctx, k8sClient, namespace, resourceRef, patchBytes)
	g.Expect(err).To(BeNil())
	actualAnnotation, err := GetResourceAnnotations(ctx, k8sClient, namespace, resourceRef)
	g.Expect(err).To(BeNil())
	for k, v := range expectedAnnotations {
		g.Expect(actualAnnotation).To(HaveKeyWithValue(k, v))
	}
}

func createAnnotationPatchBytes(g *WithT, annotMap map[string]string) []byte {
	mapJson, err := json.Marshal(annotMap)
	g.Expect(err).To(BeNil())
	b := strings.Builder{}
	b.WriteString("{\"metadata\":{\"annotations\":")
	b.WriteString(string(mapJson))
	b.WriteString("}}")
	return []byte(b.String())
}

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
