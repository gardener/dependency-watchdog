// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package coordinationv1 -destination=mocks.go k8s.io/client-go/kubernetes/typed/coordination/v1 CoordinationV1Interface,LeaseInterface
package coordinationv1
