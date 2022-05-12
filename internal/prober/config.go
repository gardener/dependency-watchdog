package prober

import (
	"io/ioutil"
	"time"

	"github.com/goccy/go-yaml"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

const (
	ScaleUp int = iota
	ScaleDown
	DefaultProbeInterval                             = 10 * time.Second
	DefaultInitialDelay                              = 30 * time.Second
	DefaultInternalProbeFailureBackoffDuration       = 30 * time.Second
	DefaultSuccessThreshold                          = 1
	DefaultFailureThreshold                          = 3
	DefaultBackoffJitterFactor                       = 0.2
	DefaultScaleUpReplicas                     int32 = 1
	DefaultScaleDownReplicas                   int32 = 0
	DefaultScaleUpdateTimeout                        = 30 * time.Second
)

type Config struct {
	InternalKubeConfigSecretName        string                  `yaml:"internalKubeConfigSecretName"`
	ExternalKubeConfigSecretName        string                  `yaml:"externalKubeConfigSecretName"`
	ProbeInterval                       *time.Duration          `yaml:"probeInterval,omitempty"`
	InitialDelay                        *time.Duration          `yaml:"initialDelay,omitempty"`
	SuccessThreshold                    *int                    `yaml:"successThreshold,omitempty"`
	FailureThreshold                    *int                    `yaml:"failureThreshold,omitempty"`
	InternalProbeFailureBackoffDuration *time.Duration          `yaml:"internalProbeFailureBackoffDuration,omitempty"`
	BackoffJitterFactor                 *float64                `yaml:"backoffJitterFactor,omitempty"`
	DependentResourceInfos              []DependentResourceInfo `yaml:"dependentResourceInfos"`
}

type DependentResourceInfo struct {
	// Ref identifies a resource
	Ref           *autoscalingv1.CrossVersionObjectReference `yaml:"ref"`
	ScaleUpInfo   *ScaleInfo                                 `yaml:"scaleUp,omitempty"`
	ScaleDownInfo *ScaleInfo                                 `yaml:"scaleDown,omitempty"`
}

type ScaleInfo struct {
	// Level is used to order the dependent resources. Highest level or the first level starts at 0 and increments. Each dependent resource on a level will have to wait for
	// all resource in a previous level to finish their scaling operation. If there are more than one resource defined with the same level then they will be scaled concurrently.
	Level int `yaml:"level"`
	// InitialDelay is the time to delay (duration) the scale down/up of this resource. If not specified its default value will be 0.
	InitialDelay *time.Duration `yaml:"initialDelay,omitempty"`
	// ScaleTimeout is the time timeout duration to wait for when attempting to update the scaling sub-resource.
	Timeout *time.Duration `yaml:"timeout,omitempty"`
	// Replicas is the desired set of replicas. In case of scale down it represents the replicas to which it should scale down. If not specified its default value will be 0.
	// In case of a scale up it represents the replicas to which it should scale up to. If not specified its default value will be 1.
	Replicas *int32 `yaml:"replicas,omitempty"`
}

func LoadConfig(file string) (*Config, error) {
	config, err := readAndUnmarshal(file)
	if err != nil {
		return nil, err
	}
	config.fillDefaultValues()
	err = config.validate()
	if err != nil {
		return nil, err
	}
	return config, nil
}

func readAndUnmarshal(file string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	config := Config{}
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Config) validate() error {
	v := new(validator)
	// Check the mandatory config parameters for which a default will not be set
	v.mustNotBeEmpty("InternalKubeConfigSecretName", c.InternalKubeConfigSecretName)
	v.mustNotBeEmpty("ExternalKubeConfigSecretName", c.ExternalKubeConfigSecretName)
	v.mustNotBeEmpty("ScaleResourceInfos", c.DependentResourceInfos)
	for _, resInfo := range c.DependentResourceInfos {
		v.resourceRefMustBeValid(resInfo.Ref)
		v.mustNotBeNil("scaleUp", resInfo.ScaleUpInfo)
		v.mustNotBeNil("scaleDown", resInfo.ScaleDownInfo)
	}
	if v.error != nil {
		return v.error
	}
	return nil
}

func (c *Config) fillDefaultValues() {
	if c.ProbeInterval == nil {
		c.ProbeInterval = new(time.Duration)
		*c.ProbeInterval = DefaultProbeInterval
	}
	if c.InitialDelay == nil {
		c.InitialDelay = new(time.Duration)
		*c.InitialDelay = DefaultInitialDelay
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

func fillDefaultValuesForResourceInfos(resourceInfos []DependentResourceInfo) {
	for _, resInfo := range resourceInfos {
		fillDefaultValuesForScaleInfo(ScaleUp, resInfo.ScaleUpInfo)
		fillDefaultValuesForScaleInfo(ScaleDown, resInfo.ScaleDownInfo)
	}
}

func fillDefaultValuesForScaleInfo(scaleType int, scaleInfo *ScaleInfo) {
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
			*scaleInfo.InitialDelay = DefaultInitialDelay
		}
	}
}

func getDefaultScaleTargetReplicas(scaleType int) int32 {
	if scaleType == ScaleUp {
		return DefaultScaleUpReplicas
	}
	return DefaultScaleDownReplicas
}
