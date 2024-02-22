// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package scaler

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

var (
	timeout  = 10 * time.Millisecond
	interval = 2 * time.Millisecond
)

func TestWithDependentResourceCheckTimeout(t *testing.T) {
	g := NewWithT(t)
	opts := scalerOptions{}
	fn := withResourceCheckTimeout(timeout)
	fn(&opts)
	g.Expect(*opts.resourceCheckTimeout).To(Equal(timeout))
}

func TestWithDependentResourceCheckInterval(t *testing.T) {
	g := NewWithT(t)
	opts := scalerOptions{}
	fn := withResourceCheckInterval(interval)
	fn(&opts)
	g.Expect(*opts.resourceCheckInterval).To(Equal(interval))
}

func TestWithScaleResourceBackOff(t *testing.T) {
	g := NewWithT(t)
	opts := scalerOptions{}
	fn := withScaleResourceBackOff(interval)
	fn(&opts)
	g.Expect(*opts.scaleResourceBackOff).To(Equal(interval))
}

func TestBuildScalerOptions(t *testing.T) {
	g := NewWithT(t)
	opts := buildScalerOptions(withResourceCheckTimeout(timeout), withResourceCheckInterval(interval))
	g.Expect(*opts.resourceCheckInterval).To(Equal(interval))
	g.Expect(*opts.resourceCheckTimeout).To(Equal(timeout))
}

func TestBuildScalerOptionsShouldFillDefaultValues(t *testing.T) {
	g := NewWithT(t)
	opts := buildScalerOptions()
	g.Expect(*opts.resourceCheckInterval).To(Equal(defaultResourceCheckInterval))
	g.Expect(*opts.resourceCheckTimeout).To(Equal(defaultResourceCheckTimeout))
}
