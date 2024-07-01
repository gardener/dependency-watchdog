// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ClientMethod is a name of the method on client.Client for which an error is recorded.
type ClientMethod string

const (
	// ClientMethodList is the name of the List method on client.Client.
	ClientMethodList ClientMethod = "List"
)

// errorRecord contains the recorded error for a specific client.Client method and identifiers such as name, namespace and matching labels.
type errorRecord struct {
	method            ClientMethod
	resourceName      string
	resourceNamespace string
	labels            labels.Set
	resourceGVK       schema.GroupVersionKind
	err               error
}

// ErrorsForGVK contains the recorded errors for a specific GroupVersionKind.
type ErrorsForGVK struct {
	GVK          schema.GroupVersionKind
	DeleteAllErr *apierrors.StatusError
	ListErr      *apierrors.StatusError
}

// FakeClientBuilder builds a client.Client which will also react to the configured errors.
type FakeClientBuilder struct {
	errorRecords    []errorRecord
	existingObjects []client.Object
	scheme          *runtime.Scheme
}

// NewFakeClientBuilder creates a new instance of FakeClientBuilder.
func NewFakeClientBuilder(existingObjects ...client.Object) *FakeClientBuilder {
	return &FakeClientBuilder{
		existingObjects: existingObjects,
	}
}

// RecordErrorForObjectsWithGVK records an error for a specific client.Client method and objects in a given namespace of a given GroupVersionKind.
func (b *FakeClientBuilder) RecordErrorForObjectsWithGVK(method ClientMethod, namespace string, gvk schema.GroupVersionKind, err *apierrors.StatusError) *FakeClientBuilder {
	// this method records error, so if nil error is passed then there is no need to create any error record.
	if err == nil {
		return b
	}
	b.errorRecords = append(b.errorRecords, errorRecord{
		method:            method,
		resourceGVK:       gvk,
		resourceNamespace: namespace,
		err:               err,
	})
	return b
}

// WithScheme sets the scheme for the client.
func (b *FakeClientBuilder) WithScheme(scheme *runtime.Scheme) *FakeClientBuilder {
	b.scheme = scheme
	return b
}

// Build creates a new instance of client.Client which will react to the configured errors.
func (b *FakeClientBuilder) Build() client.Client {
	if b.scheme == nil {
		b.scheme = scheme.Scheme
	}
	return &fakeClient{
		Client:       fake.NewClientBuilder().WithObjects(b.existingObjects...).WithScheme(b.scheme).Build(),
		errorRecords: b.errorRecords,
	}
}

// fakeClient is a client.Client implementation which reacts to the configured errors.
type fakeClient struct {
	client.Client
	errorRecords []errorRecord
}

// ---------------------------------- Implementation of client.Client ----------------------------------

func (c *fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	gvk, err := apiutil.GVKForObject(list, c.Scheme())
	if err != nil {
		return err
	}

	if err := c.getRecordedObjectCollectionError(ClientMethodList, listOpts.Namespace, listOpts.LabelSelector, gvk); err != nil {
		return err
	}
	return c.Client.List(ctx, list, opts...)
}

// ---------------------------------- Helper methods ----------------------------------

func (c *fakeClient) getRecordedObjectCollectionError(method ClientMethod, namespace string, labelSelector labels.Selector, objGVK schema.GroupVersionKind) error {
	for _, errRecord := range c.errorRecords {
		if errRecord.method == method && errRecord.resourceNamespace == namespace {
			if errRecord.resourceGVK == objGVK || (labelSelector == nil && errRecord.labels == nil) || labelSelector.Matches(errRecord.labels) {
				return errRecord.err
			}
		}
	}
	return nil
}
