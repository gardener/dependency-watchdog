// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package scaler -destination=mocks.go github.com/gardener/dependency-watchdog/internal/prober/scaler Scaler
package scaler
