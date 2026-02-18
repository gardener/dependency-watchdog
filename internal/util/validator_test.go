// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package util

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMustNotBeEmpty(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		key    string
		value  any
		result bool
	}{
		{"", nil, false},
		{"k1", "  ", false},
		{"k2", "valid-value", true},
		{"k3", []string{}, false},
		{"k4", []string{"bingo"}, true},
		{"k5", map[string]string{}, false},
		{"k6", map[string]string{"bingo": "tringo"}, true},
		{"k7", struct{ name string }{name: "bingo"}, false},
	}

	for _, entry := range tests {
		v := Validator{}
		actualResult := v.MustNotBeEmpty(entry.key, entry.value)
		g.Expect(entry.result).To(Equal(actualResult))
		if !actualResult {
			g.Expect(v.Error).To(HaveOccurred())
		}
	}
}

func TestMustNotBeZeroDuration(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		key    string
		value  metav1.Duration
		result bool
	}{
		{"k1", metav1.Duration{}, false},
		{"k2", metav1.Duration{Duration: 100}, true},
		{"k3", metav1.Duration{Duration: 0}, false},
	}
	for _, entry := range tests {
		v := Validator{}
		actualResult := v.MustNotBeZeroDuration(entry.key, entry.value)
		g.Expect(entry.result).To(Equal(actualResult))
		if !actualResult {
			g.Expect(v.Error).To(HaveOccurred())
		}
	}
}

func TestMustNotBeNil(t *testing.T) {
	g := NewWithT(t)
	var ch chan struct{}
	tests := []struct {
		key    string
		value  any
		result bool
	}{
		{"k1", nil, false},
		{"k2", ch, false},
		{"k3", []string{}, true},
	}

	for _, entry := range tests {
		v := Validator{}
		actualResult := v.MustNotBeNil(entry.key, entry.value)
		g.Expect(entry.result).To(Equal(actualResult))
		if !actualResult {
			g.Expect(v.Error).To(HaveOccurred())
		}
	}
}

func TestResourceRefMustBeValid(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		resourceRef autoscalingv1.CrossVersionObjectReference
		result      bool
	}{
		{autoscalingv1.CrossVersionObjectReference{Kind: "", Name: "", APIVersion: ""}, false},
		{autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "d1", APIVersion: "apps/v1"}, true},
		{autoscalingv1.CrossVersionObjectReference{Kind: "ConfigMap", Name: "c1", APIVersion: "v1"}, true},
		{autoscalingv1.CrossVersionObjectReference{Kind: "StatefulSet", Name: "s1", APIVersion: "v1"}, false},
		{autoscalingv1.CrossVersionObjectReference{Kind: "Depoyment", Name: "d2", APIVersion: "apps/v1"}, false},
		{autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "d2", APIVersion: "core/apps/v1"}, false},
	}

	for _, entry := range tests {
		v := Validator{}
		actualResult := v.ResourceRefMustBeValid(&entry.resourceRef, scheme)
		g.Expect(entry.result).To(Equal(actualResult))
	}
}
