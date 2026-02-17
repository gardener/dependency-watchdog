// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package util

import (
	"testing"

	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsNodeUndergoingInPlaceUpdate(t *testing.T) {
	tests := []struct {
		description    string
		nodeConditions []corev1.NodeCondition
		expected       bool
	}{
		{
			description: "should return true when node has InPlaceUpdate condition with ReadyForUpdate reason",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   v1alpha1.NodeInPlaceUpdate,
					Status: corev1.ConditionTrue,
					Reason: v1alpha1.ReadyForUpdate,
				},
			},
			expected: true,
		},
		{
			description: "should return true when node has InPlaceUpdate condition with UpdateFailed reason",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   v1alpha1.NodeInPlaceUpdate,
					Status: corev1.ConditionTrue,
					Reason: v1alpha1.UpdateFailed,
				},
			},
			expected: true,
		},
		{
			description: "should return true when node has InPlaceUpdate condition with ReadyForUpdate reason among other conditions",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   v1alpha1.NodeInPlaceUpdate,
					Status: corev1.ConditionTrue,
					Reason: v1alpha1.ReadyForUpdate,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
			expected: true,
		},
		{
			description: "should return true when node has InPlaceUpdate condition with UpdateFailed reason among other conditions",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   v1alpha1.NodeInPlaceUpdate,
					Status: corev1.ConditionTrue,
					Reason: v1alpha1.UpdateFailed,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
			expected: true,
		},
		{
			description: "should return false when node has InPlaceUpdate condition with different reason",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   v1alpha1.NodeInPlaceUpdate,
					Status: corev1.ConditionTrue,
					Reason: v1alpha1.CandidateForUpdate,
				},
			},
			expected: false,
		},
		{
			description: "should return false when node has no InPlaceUpdate condition",
			nodeConditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
			expected: false,
		},
		{
			description:    "should return false when node has no conditions",
			nodeConditions: []corev1.NodeCondition{},
			expected:       false,
		},
		{
			description:    "should return false when node has nil conditions",
			nodeConditions: nil,
			expected:       false,
		},
	}

	t.Parallel()
	g := NewWithT(t)
	for _, test := range tests {
		t.Run(test.description, func(_ *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Conditions: test.nodeConditions,
				},
			}
			result := IsNodeUndergoingInPlaceUpdate(node)
			g.Expect(result).To(Equal(test.expected))
		})
	}
}
