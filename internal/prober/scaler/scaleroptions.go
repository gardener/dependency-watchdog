// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"time"

	"k8s.io/utils/pointer"
)

const (
	defaultResourceCheckTimeout  = 5 * time.Second
	defaultResourceCheckInterval = 1 * time.Second
	defaultScaleResourceBackoff  = 100 * time.Millisecond
)

type scalerOption func(options *scalerOptions)

type scalerOptions struct {
	resourceCheckTimeout  *time.Duration
	resourceCheckInterval *time.Duration
	scaleResourceBackOff  *time.Duration
}

func buildScalerOptions(options ...scalerOption) *scalerOptions {
	opts := new(scalerOptions)
	for _, opt := range options {
		opt(opts)
	}
	fillDefaultsOptions(opts)
	return opts
}

func withResourceCheckTimeout(timeout time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.resourceCheckTimeout = &timeout
	}
}

func withResourceCheckInterval(interval time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.resourceCheckInterval = &interval
	}
}

func withScaleResourceBackOff(interval time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.scaleResourceBackOff = &interval
	}
}

func fillDefaultsOptions(options *scalerOptions) {
	if options.resourceCheckTimeout == nil {
		options.resourceCheckTimeout = pointer.Duration(defaultResourceCheckTimeout)
	}
	if options.resourceCheckInterval == nil {
		options.resourceCheckInterval = pointer.Duration(defaultResourceCheckInterval)
	}
	if options.scaleResourceBackOff == nil {
		options.scaleResourceBackOff = pointer.Duration(defaultScaleResourceBackoff)
	}
}
