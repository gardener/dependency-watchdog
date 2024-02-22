// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package scale -destination=mocks.go k8s.io/client-go/scale ScaleInterface
package scale
