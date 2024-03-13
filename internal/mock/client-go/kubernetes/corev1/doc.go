// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package corev1 -destination=mocks.go k8s.io/client-go/kubernetes/typed/core/v1 CoreV1Interface,NodeInterface
package corev1
