// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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

// CreateTestNamespace creates a namespace with the given namePrefix
func CreateTestNamespace(ctx context.Context, g *WithT, cli client.Client, name string) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	g.Expect(cli.Create(ctx, &ns)).To(Succeed())
}

// TeardownEnv cancels the context and stops testenv.
func TeardownEnv(g *WithT, testEnv *envtest.Environment, cancelFn context.CancelFunc) {
	cancelFn()
	err := testEnv.Stop()
	g.Expect(err).NotTo(HaveOccurred())
}
