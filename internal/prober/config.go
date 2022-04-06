package prober

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v3"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

const (
	ScaleUp int = iota
	ScaleDown
)

var (
	defaultProbeInterval             = 10 * time.Second
	defaultInitialDelay              = 0 * time.Second
	defaultBackoffDuration           = 30 * time.Second
	defaultSuccessThreshold          = 1
	defaultFailureThreshold          = 3
	defaultBackoffJitterFactor       = 0.2
	defaultScaleUpReplicas     int32 = 1
	defaultScaleDownReplicas   int32 = 0
	defaultScaleUpdateTimeout        = 10 * time.Second
)

type Config struct {
	Name                         string         `yaml:"name"`
	Namespace                    string         `yaml:"namespace,omitempty"`
	InternalKubeConfigSecretName string         `yaml:"internalKubeConfigSecretName"`
	ExternalKubeConfigSecretName string         `yaml:"externalKubeConfigSecretName"`
	ProbeInterval                *time.Duration `yaml:"probeInterval,omitempty"`
	InitialDelay                 *time.Duration `yaml:"initialDelay,omitempty"`
	SuccessThreshold             *int           `yaml:"successThreshold,omitempty"`
	FailureThreshold             *int           `yaml:"failureThreshold,omitempty"`
	BackoffDuration              *time.Duration `yaml:"backoffDuration,omitempty"`
	BackoffJitterFactor          *float64       `yaml:"backoffJitterFactor,omitempty"`
	ScaleDownResourceInfos       []ResourceInfo `yaml:"scaleDownResourceInfos"`
	ScaleUpResourceInfos         []ResourceInfo `yaml:"scaleUpResourceInfos"`
}

type ResourceInfo struct {
	// Ref identifies a resource
	Ref autoscalingv1.CrossVersionObjectReference `yaml:"ref"`
	// Level is used to order the dependent resources. Highest level or the first level starts at 0 and increments. Each dependent resource on a level will have to wait for
	// all resource in a previous level to finish their scaling operation. If there are more than one resource defined with the same level then they will be scaled concurrently.
	Level int `yaml:"level"`
	// InitialDelay is the time to delay (duration) the scale down/up of this resource. If not specified its default value will be 0.
	InitialDelay *time.Duration `yaml:"initialDelay,omitempty"`
	// ScaleTimeout is the time timeout duration to wait for when attempting to update the scaling sub-resource.
	ScaleUpdateTimeout *time.Duration `yaml:"scaleUpdateTimeout,omitempty"`
	// Replicas is the desired set of replicas. In case of scale down it represents the replicas to which it should scale down. If not specified its default value will be 0.
	// In case of a scale up it represents the replicas to which it should scale up to. If not specified its default value will be 1.
	Replicas *int32 `yaml:"replicas,omitempty"`
}

func ReadAndUnmarshal(file string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	config := Config{}
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}
	config.fillDefaultValues()
	err = config.validate()
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *Config) validate() error {
	v := new(validator)
	// Check the mandatory config parameters for which a default will not be set
	v.MustNotBeEmpty("Name", c.Name)
	v.MustNotBeEmpty("InternalKubeConfigSecretName", c.InternalKubeConfigSecretName)
	v.MustNotBeEmpty("ExternalKubeConfigSecretName", c.ExternalKubeConfigSecretName)
	v.MustNotBeEmpty("ScaleDownResourceInfos", c.ScaleDownResourceInfos)
	v.MustNotBeEmpty("ScaleUpResourceInfos", c.ScaleUpResourceInfos)
	for _, resInfo := range c.ScaleUpResourceInfos {
		v.ResourceRefMustBeValid(resInfo.Ref)
	}
	for _, resInfo := range c.ScaleDownResourceInfos {
		v.ResourceRefMustBeValid(resInfo.Ref)
	}
	if v.error != nil {
		return v.error
	}
	return nil
}

func (c *Config) fillDefaultValues() {
	if c.ProbeInterval == nil {
		c.ProbeInterval = &defaultProbeInterval
	}
	if c.InitialDelay == nil {
		c.InitialDelay = &defaultInitialDelay
	}
	if c.BackoffDuration == nil {
		c.BackoffDuration = &defaultBackoffDuration
	}
	if c.SuccessThreshold == nil {
		c.SuccessThreshold = &defaultSuccessThreshold
	}
	if c.FailureThreshold == nil {
		c.FailureThreshold = &defaultFailureThreshold
	}
	if c.BackoffJitterFactor == nil {
		c.BackoffJitterFactor = &defaultBackoffJitterFactor
	}
	fillDefaultValuesForResourceInfos(ScaleUp, c.ScaleUpResourceInfos)
	fillDefaultValuesForResourceInfos(ScaleDown, c.ScaleDownResourceInfos)
}

func fillDefaultValuesForResourceInfos(scaleType int, resourceInfos []ResourceInfo) {
	for _, resInfo := range resourceInfos {
		if resInfo.Replicas == nil {
			resInfo.Replicas = getDefaultScaleTargetReplicas(scaleType)
		}
		if resInfo.ScaleUpdateTimeout == nil {
			resInfo.ScaleUpdateTimeout = &defaultScaleUpdateTimeout
		}
		if resInfo.InitialDelay == nil {
			resInfo.InitialDelay = &defaultInitialDelay
		}
	}
}

func getDefaultScaleTargetReplicas(scaleType int) *int32 {
	if scaleType == ScaleUp {
		return &defaultScaleUpReplicas
	}
	return &defaultScaleDownReplicas
}

func (c *Config) GetSecretNames() []string {
	secretNames := make([]string, 2)
	// it is assumed that mandatory check will already been done in validate method, so just collect the secret names
	secretNames = append(secretNames, c.InternalKubeConfigSecretName)
	secretNames = append(secretNames, c.ExternalKubeConfigSecretName)
	return secretNames
}
