package util_test

import (
	"bytes"
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/gardener/dependency-watchdog/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	pathToTestDirectory = filepath.Join("..", "..", "testdata")
	pathToSecret        = filepath.Join(pathToTestDirectory, "secret.yaml")
	pathToKubeConfig    = filepath.Join(pathToTestDirectory, "kubeconfig.yaml")
	pathToDeployment    = filepath.Join(pathToTestDirectory, "deployment.yaml")
)

var _ = Describe("Client", Ordered, Label("client"), func() {
	var (
		testEnv   *envtest.Environment
		k8sClient client.Client
		cfg       *rest.Config
		err       error
	)

	BeforeAll(func() {
		By("initialing and starting the test environment")
		testEnv = &envtest.Environment{}
		cfg, err = testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		By("creating a new k8s client")
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())
	})

	Describe("GetKubeConfigFromSecret", func() {
		var ctx context.Context
		var secret corev1.Secret

		BeforeEach(func() {
			ctx = context.Background()
			result := getStructured[corev1.Secret](pathToSecret)
			Expect(result.Err).Should(BeNil())
			Expect(result.StructuredObject).ShouldNot(BeNil())
			secret = result.StructuredObject
		})

		It("should return error if secret is not found", func() {
			kubeconfig, err := util.GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
			Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			Expect(kubeconfig).Should(BeNil())
		})

		Describe("Found the secret", func() {
			It("should return error if kubeconfig is not found", func() {
				err := k8sClient.Create(ctx, &secret)
				Expect(err).Should(BeNil())
				kubeconfig, err := util.GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
				Expect(kubeconfig).Should(BeNil())
				Expect(err).ShouldNot(BeNil())
				Expect(apierrors.IsNotFound(err)).Should(BeFalse())
			})

			It("should extract kubeconfig from secret", func() {
				By("reading the kubeconfig file")
				kubeconfigBuffer, err := readFile(pathToKubeConfig)
				Expect(err).Should(BeNil())
				kubeconfig := kubeconfigBuffer.Bytes()
				Expect(kubeconfig).ShouldNot(BeNil())

				By("creating the secret")
				secret.Data = map[string][]byte{
					"kubeconfig": kubeconfig,
				}
				err = k8sClient.Create(ctx, &secret)
				Expect(err).Should(BeNil())

				By("matching the returned kubeconfig with the deployed one")
				actualKubeconfig, err := util.GetKubeConfigFromSecret(ctx, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, k8sClient)
				Expect(err).Should(BeNil())
				Expect(actualKubeconfig).Should(Equal(kubeconfig))
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, &secret)
				Expect(err).Should(BeNil())
			})
		})
	})

	Describe("GetDeploymentFor", Label("client"), func() {
		var ctx context.Context
		var deployment appsv1.Deployment

		BeforeEach(func() {
			ctx = context.Background()

			result := getStructured[appsv1.Deployment](pathToDeployment)
			Expect(result.Err).Should(BeNil())
			Expect(result.StructuredObject).ShouldNot(BeNil())
			deployment = result.StructuredObject
		})

		It("should return error if deployment is not found", func() {
			actual, err := util.GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient)
			Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			Expect(actual).Should(BeNil())
		})

		It("should return the deployment if found", func() {
			err := k8sClient.Create(ctx, &deployment)
			Expect(err).Should(BeNil())

			actual, err := util.GetDeploymentFor(ctx, deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, k8sClient)
			Expect(err).Should(BeNil())
			Expect(actual).ShouldNot(BeNil())
			Expect(actual.ObjectMeta.Name).Should(Equal(deployment.ObjectMeta.Name))
			Expect(actual.ObjectMeta.Namespace).Should(Equal(deployment.ObjectMeta.Namespace))

			err = k8sClient.Delete(ctx, &deployment)
			Expect(err).Should(BeNil())
		})
	})

	Describe("CreateScalesGetter", Label("client"), func() {
		It("should return scaleGetter if config is ok", func() {
			scaleGetter, err := util.CreateScalesGetter(cfg)
			Expect(err).Should(BeNil())
			Expect(scaleGetter).ShouldNot(BeNil())
		})
	})

	Describe("Create ClientFromKubeConfigBytes", Label("client"), func() {
		It("should return new client from kubeconfig", func() {
			By("reading the kubeconfig file")
			kubeconfigBuffer, err := readFile(pathToKubeConfig)
			Expect(err).Should(BeNil())
			kubeconfig := kubeconfigBuffer.Bytes()
			Expect(kubeconfig).ShouldNot(BeNil())

			cfg, err := util.CreateClientFromKubeConfigBytes(kubeconfig)
			Expect(err).Should(BeNil())
			Expect(cfg).ShouldNot(BeNil())
		})
	})

	AfterAll(func() {
		By("tearing down the test environment")
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	})

})

type Result[T any] struct {
	StructuredObject T
	Err              error
}

func getStructured[T any](filepath string) Result[T] {
	unstructuredObject, err := getUnstructured(filepath)
	if err != nil {
		return Result[T]{Err: err}
	}

	var structuredObject T
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObject.Object, &structuredObject)
	if err != nil {
		return Result[T]{Err: err}
	}

	return Result[T]{StructuredObject: structuredObject}
}

func getUnstructured(filePath string) (*unstructured.Unstructured, error) {
	buff, err := readFile(filePath)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	jsonObject, err := yaml.ToJSON(buff.Bytes())
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsonObject)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	unstructuredObject, ok := object.(*unstructured.Unstructured)
	if !ok {
		return &unstructured.Unstructured{}, fmt.Errorf("unstructured.Unstructured expected")
	}

	return unstructuredObject, nil
}

func readFile(filePath string) (*bytes.Buffer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(file)
	if err != nil {
		return nil, err
	}
	return buff, nil
}
