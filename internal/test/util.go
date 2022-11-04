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

package test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// GetStructured reads the file present at the given filePath and returns a structured object based on the type T.
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

// GetUnstructured reads the file present at the given filePath and returns an unstructured.Unstructured object from its contents.
func GetUnstructured(filePath string) (*unstructured.Unstructured, error) {
	buff, err := ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	jsonObject, err := yaml.ToJSON(buff.Bytes())
	if err != nil {
		return nil, err
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsonObject)
	if err != nil {
		return nil, err
	}
	unstructuredObject, ok := object.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("unstructured.Unstructured expected")
	}
	return unstructuredObject, nil
}

// ReadFile reads the file present at the given filePath and returns a byte Buffer containing its contents.
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

// FileExistsOrFail checks if the given filepath is valid and returns an error if file is not found or does not exist.
func FileExistsOrFail(filepath string) {
	var err error
	if _, err = os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", filepath)
	}
	if err != nil {
		log.Fatalf("Error occured in finding file %s : %v", filepath, err)
	}
}

// ValidateIfFileExists validates the existence of a file
func ValidateIfFileExists(file string, t *testing.T) {
	g := gomega.NewWithT(t)
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(gomega.HaveOccurred(), "File at path %v should exist")
}
