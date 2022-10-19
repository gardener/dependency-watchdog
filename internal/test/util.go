package test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func GetStructured[T any](filepath string) (*T, error) {
	unstructuredObject, err := GetUnstructured(filepath)
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

func GetUnstructured(filePath string) (*unstructured.Unstructured, error) {
	buff, err := ReadFile(filePath)
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

func ReadFile(filePath string) (*bytes.Buffer, error) {
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

func FileExistsOrFail(filepath string) {
	var err error
	if _, err = os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", filepath)
	}
	if err != nil {
		log.Fatalf("Error occured in finding file %s : %v", filepath, err)
	}
}
