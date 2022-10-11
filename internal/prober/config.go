package prober

import (
	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
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
	config, err := util.ReadAndUnmarshall[papi.Config](file)
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

func validate(c *papi.Config) error {
	v := new(util.Validator)
	// Check the mandatory config parameters for which a default will not be set
	v.MustNotBeEmpty("InternalKubeConfigSecretName", c.InternalKubeConfigSecretName)
	v.MustNotBeEmpty("ExternalKubeConfigSecretName", c.ExternalKubeConfigSecretName)
	v.MustNotBeEmpty("ScaleResourceInfos", c.DependentResourceInfos)
	for _, resInfo := range c.DependentResourceInfos {
		v.ResourceRefMustBeValid(resInfo.Ref)
		v.MustNotBeNil("shouldExist", resInfo.ShouldExist)
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

func getDefaultScaleTargetReplicas(scaleType int) int32 {
	if scaleType == ScaleUp {
		return DefaultScaleUpReplicas
	}
	return DefaultScaleDownReplicas
}
