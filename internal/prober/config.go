package prober

import (
	"io/ioutil"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/goccy/go-yaml"
)

const (
	ScaleUp int = iota
	ScaleDown
	DefaultProbeInterval                             = 10 * time.Second
	DefaultProbeInitialDelay                         = 30 * time.Second
	DefaultScaleInitialDelay                         = 0 * time.Second
	DefaultProbeTimeout                              = 30 * time.Second
	DefaultInternalProbeFailureBackoffDuration       = 30 * time.Second
	DefaultSuccessThreshold                          = 1
	DefaultFailureThreshold                          = 3
	DefaultBackoffJitterFactor                       = 0.2
	DefaultScaleUpReplicas                     int32 = 1
	DefaultScaleDownReplicas                   int32 = 0
	DefaultScaleUpdateTimeout                        = 30 * time.Second
)

func LoadConfig(file string) (*papi.Config, error) {
	config, err := readAndUnmarshal(file)
	if err != nil {
		return nil, err
	}
	fillDefaultValues(config)
	err = validate(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func readAndUnmarshal(file string) (*papi.Config, error) {
	configBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	config := papi.Config{}
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func validate(c *papi.Config) error {
	v := new(validator)
	// Check the mandatory config parameters for which a default will not be set
	v.mustNotBeEmpty("InternalKubeConfigSecretName", c.InternalKubeConfigSecretName)
	v.mustNotBeEmpty("ExternalKubeConfigSecretName", c.ExternalKubeConfigSecretName)
	v.mustNotBeEmpty("ScaleResourceInfos", c.DependentResourceInfos)
	for _, resInfo := range c.DependentResourceInfos {
		v.resourceRefMustBeValid(resInfo.Ref)
		v.mustNotBeNil("shouldExist", resInfo.ShouldExist)
		v.mustNotBeNil("scaleUp", resInfo.ScaleUpInfo)
		v.mustNotBeNil("scaleDown", resInfo.ScaleDownInfo)
	}
	if v.error != nil {
		return v.error
	}
	return nil
}

func fillDefaultValues(c *papi.Config) {
	if c.ProbeInterval == nil {
		c.ProbeInterval = new(time.Duration)
		*c.ProbeInterval = DefaultProbeInterval
	}
	if c.InitialDelay == nil {
		c.InitialDelay = new(time.Duration)
		*c.InitialDelay = DefaultProbeInitialDelay
	}
	if c.ProbeTimeout == nil {
		c.ProbeTimeout = new(time.Duration)
		*c.ProbeTimeout = DefaultProbeTimeout
	}
	if c.InternalProbeFailureBackoffDuration == nil {
		c.InternalProbeFailureBackoffDuration = new(time.Duration)
		*c.InternalProbeFailureBackoffDuration = DefaultInternalProbeFailureBackoffDuration
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
		fillDefaultValuesForScaleInfo(ScaleUp, resInfo.ScaleUpInfo)
		fillDefaultValuesForScaleInfo(ScaleDown, resInfo.ScaleDownInfo)
	}
}

func fillDefaultValuesForScaleInfo(scaleType int, scaleInfo *papi.ScaleInfo) {
	if scaleInfo != nil {
		if scaleInfo.Replicas == nil {
			scaleInfo.Replicas = new(int32)
			*scaleInfo.Replicas = getDefaultScaleTargetReplicas(scaleType)
		}
		if scaleInfo.Timeout == nil {
			scaleInfo.Timeout = new(time.Duration)
			*scaleInfo.Timeout = DefaultScaleUpdateTimeout
		}
		if scaleInfo.InitialDelay == nil {
			scaleInfo.InitialDelay = new(time.Duration)
			*scaleInfo.InitialDelay = DefaultScaleInitialDelay
		}
	}
}

func getDefaultScaleTargetReplicas(scaleType int) int32 {
	if scaleType == ScaleUp {
		return DefaultScaleUpReplicas
	}
	return DefaultScaleDownReplicas
}
