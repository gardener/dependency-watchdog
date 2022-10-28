// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scaler

import (
	"time"

	"k8s.io/utils/pointer"
)

const (
	defaultDependentResourceCheckTimeout  = 20 * time.Millisecond
	defaultDependentResourceCheckInterval = 5 * time.Millisecond
	defaultScaleResourceBackoff           = 100 * time.Millisecond
)

type scalerOption func(options *scalerOptions)

type scalerOptions struct {
	dependentResourceCheckTimeout  *time.Duration
	dependentResourceCheckInterval *time.Duration
	scaleResourceBackOff           *time.Duration
}

func buildScalerOptions(options ...scalerOption) *scalerOptions {
	opts := new(scalerOptions)
	for _, opt := range options {
		opt(opts)
	}
	fillDefaultsOptions(opts)
	return opts
}

func withDependentResourceCheckTimeout(timeout time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.dependentResourceCheckTimeout = &timeout
	}
}

func withDependentResourceCheckInterval(interval time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.dependentResourceCheckInterval = &interval
	}
}

func withScaleResourceBackOff(interval time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.scaleResourceBackOff = &interval
	}
}

func fillDefaultsOptions(options *scalerOptions) {
	if options.dependentResourceCheckTimeout == nil {
		options.dependentResourceCheckTimeout = pointer.Duration(defaultDependentResourceCheckTimeout)
	}
	if options.dependentResourceCheckInterval == nil {
		options.dependentResourceCheckInterval = pointer.Duration(defaultDependentResourceCheckInterval)
	}
	if options.scaleResourceBackOff == nil {
		options.scaleResourceBackOff = pointer.Duration(defaultScaleResourceBackoff)
	}
}
