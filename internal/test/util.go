// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(f)
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
	g := NewWithT(t)
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(HaveOccurred(), "File at path %v should exist")
}

// CreateTestNamespace creates a namespace with the given namePrefix
func CreateTestNamespace(ctx context.Context, g *WithT, cli client.Client, namePrefix string) string {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix + "-",
		},
	}
	g.Expect(cli.Create(ctx, &ns)).To(Succeed())
	return ns.Name
}

// TeardownEnv cancels the context and stops testenv.
func TeardownEnv(g *WithT, testEnv *envtest.Environment, cancelFn context.CancelFunc) {
	cancelFn()
	err := testEnv.Stop()
	g.Expect(err).NotTo(HaveOccurred())
}

// MergeMaps merges newMaps with an oldMap. Keys defined in the new Map which are present in the old Map will be overwritten.
func MergeMaps[T any](oldMap map[string]T, newMaps ...map[string]T) map[string]T {
	var out map[string]T

	if oldMap != nil {
		out = make(map[string]T)
	}
	for k, v := range oldMap {
		out[k] = v
	}

	for _, newMap := range newMaps {
		if newMap != nil && out == nil {
			out = make(map[string]T)
		}

		for k, v := range newMap {
			out[k] = v
		}
	}

	return out
}
