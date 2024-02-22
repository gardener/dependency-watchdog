// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate mockgen -package prober -destination=mocks.go github.com/gardener/dependency-watchdog/internal/prober ShootClientCreator
package prober
