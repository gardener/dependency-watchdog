package prober

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func BeforeSuite(t *testing.T) (client.Client, *rest.Config, *envtest.Environment) {
	// t.Log("setting up envTest")
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		log.Fatalf("error in starting testEnv: %v", err)
	}
	if cfg == nil {
		log.Fatalf("Got nil config from testEnv")
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Fatalf("error in creating new client: %v", err)
	}
	if k8sClient == nil {
		log.Fatalf("Got a nil k8sClient")
	}
	return k8sClient, cfg, testEnv
}

func AfterSuite(t *testing.T, testEnv *envtest.Environment) {
	log.Println("tearing down envTest")
	err := testEnv.Stop()
	if err != nil {
		log.Fatalf("error in stopping testEnv: %v", err)
	}
}

type Result[T any] struct {
	StructuredObject T
	Err              error
}

func getStructured[T any](filepath string) (*T, error) {
	unstructuredObject, err := getUnstructured(filepath)
	if err != nil {
		return nil, err
	}
	var structuredObject T
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObject.Object, &structuredObject)
	if err != nil {
		return nil, err
	}
	return &structuredObject, nil
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

func fileExistsOrFail(filepath string) {
	var err error
	if _, err = os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", filepath)
	}
	if err != nil {
		log.Fatalf("Error occured in finding file %s : %v", filepath, err)
	}
}
