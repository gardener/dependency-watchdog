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

package prober

import (
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// DefaultProbeInterval is the default duration representing the interval for a probe.
	DefaultProbeInterval = 10 * time.Second
	// DefaultProbeInitialDelay is the default duration representing an initial delay to start a probe.
	DefaultProbeInitialDelay = 30 * time.Second
	// DefaultScaleInitialDelay is the default duration representing an initial delay to start to scale up a kubernetes resource.
	DefaultScaleInitialDelay = 0 * time.Second
	// DefaultProbeTimeout is the default duration representing total timeout for a probe to complete.
	DefaultProbeTimeout = 30 * time.Second
	// DefaultInternalProbeFailureBackoffDuration is the default duration representing a backOff duration in the event the internal probe transitions to failed state.
	DefaultInternalProbeFailureBackoffDuration = 30 * time.Second
	// DefaultSuccessThreshold is the default value for consecutive successful probes required to transition a probe status to success.
	DefaultSuccessThreshold = 1
	// DefaultFailureThreshold is the default value for consecutive erroneous probes required to transition a probe status to failed.
	DefaultFailureThreshold = 3
	// DefaultBackoffJitterFactor is the default jitter value with which successive probe runs are scheduled.
	DefaultBackoffJitterFactor = 0.2
	// DefaultScaleUpdateTimeout is the default duration representing a timeout for the scale operation to complete.
	DefaultScaleUpdateTimeout = 30 * time.Second
)

// LoadConfig reads the prober configuration from a file, unmarshalls it, fills in the default values and
// validates the unmarshalled configuration If all validations pass it will return papi.Config else it will return an error.
func LoadConfig(file string, scheme *runtime.Scheme) (*papi.Config, error) {
	config, err := util.ReadAndUnmarshall[papi.Config](file)
	if err != nil {
		return nil, err
	}
	fillDefaultValues(config)
	err = validate(config, scheme)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func validate(c *papi.Config, scheme *runtime.Scheme) error {
	v := new(util.Validator)
	// Check the mandatory config parameters for which a default will not be set
	v.MustNotBeEmpty("InternalKubeConfigSecretName", c.InternalKubeConfigSecretName)
	v.MustNotBeEmpty("ExternalKubeConfigSecretName", c.ExternalKubeConfigSecretName)
	v.MustNotBeEmpty("ScaleResourceInfos", c.DependentResourceInfos)
	for _, resInfo := range c.DependentResourceInfos {
		v.ResourceRefMustBeValid(resInfo.Ref, scheme)
		v.MustNotBeNil("optional", resInfo.Optional)
		v.MustNotBeNil("scaleUp", resInfo.ScaleUpInfo)
		v.MustNotBeNil("scaleDown", resInfo.ScaleDownInfo)
	}
	if v.Error != nil {
		return v.Error
	}
	return nil
}

func fillDefaultValues(c *papi.Config) {
	if c.ProbeInterval == nil {
		c.ProbeInterval = &metav1.Duration{
			Duration: DefaultProbeInterval,
		}
	}
	if c.InitialDelay == nil {
		c.InitialDelay = &metav1.Duration{
			Duration: DefaultProbeInitialDelay,
		}
	}
	if c.ProbeTimeout == nil {
		c.ProbeTimeout = &metav1.Duration{
			Duration: DefaultProbeTimeout,
		}
	}
	if c.InternalProbeFailureBackoffDuration == nil {
		c.InternalProbeFailureBackoffDuration = &metav1.Duration{
			Duration: DefaultInternalProbeFailureBackoffDuration,
		}
	}
	if c.SuccessThreshold == nil {
		c.SuccessThreshold = new(int)
		*c.SuccessThreshold = DefaultSuccessThreshold
	}
	if c.FailureThreshold == nil {
		c.FailureThreshold = new(int)
		*c.FailureThreshold = DefaultFailureThreshold
	}
	if c.BackoffJitterFactor == nil {
		c.BackoffJitterFactor = new(float64)
		*c.BackoffJitterFactor = DefaultBackoffJitterFactor
	}
	fillDefaultValuesForResourceInfos(c.DependentResourceInfos)
}

func fillDefaultValuesForResourceInfos(resourceInfos []papi.DependentResourceInfo) {
	for _, resInfo := range resourceInfos {
		fillDefaultValuesForScaleInfo(resInfo.ScaleUpInfo)
		fillDefaultValuesForScaleInfo(resInfo.ScaleDownInfo)
	}
}

func fillDefaultValuesForScaleInfo(scaleInfo *papi.ScaleInfo) {
	if scaleInfo != nil {
		if scaleInfo.Timeout == nil {
			scaleInfo.Timeout = &metav1.Duration{
				Duration: DefaultScaleUpdateTimeout,
			}
		}
		if scaleInfo.InitialDelay == nil {
			scaleInfo.InitialDelay = &metav1.Duration{
				Duration: DefaultScaleInitialDelay,
			}
		}
	}
}
