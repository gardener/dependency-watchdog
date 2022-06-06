package prober

import (
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

type Config struct {
	InternalKubeConfigSecretName        string                  `yaml:"internalKubeConfigSecretName"`
	ExternalKubeConfigSecretName        string                  `yaml:"externalKubeConfigSecretName"`
	ProbeInterval                       *time.Duration          `yaml:"probeInterval,omitempty"`
	InitialDelay                        *time.Duration          `yaml:"initialDelay,omitempty"`
	ProbeTimeout                        *time.Duration          `yaml:"probeTimeout,omitempty"`
	SuccessThreshold                    *int                    `yaml:"successThreshold,omitempty"`
	FailureThreshold                    *int                    `yaml:"failureThreshold,omitempty"`
	InternalProbeFailureBackoffDuration *time.Duration          `yaml:"internalProbeFailureBackoffDuration,omitempty"`
	BackoffJitterFactor                 *float64                `yaml:"backoffJitterFactor,omitempty"`
	DependentResourceInfos              []DependentResourceInfo `yaml:"dependentResourceInfos"`
}

type DependentResourceInfo struct {
	// Ref identifies a resource
	Ref           *autoscalingv1.CrossVersionObjectReference `yaml:"ref"`
	ShouldExist   *bool                                      `yaml:"shouldExist"`
	ScaleUpInfo   *ScaleInfo                                 `yaml:"scaleUp,omitempty"`
	ScaleDownInfo *ScaleInfo                                 `yaml:"scaleDown,omitempty"`
}

type ScaleInfo struct {
	// Level is used to order the dependent resources. Highest level or the first level starts at 0 and increments. Each dependent resource on a level will have to wait for
	// all resource in a previous level to finish their scaling operation. If there are more than one resource defined with the same level then they will be scaled concurrently.
	Level int `yaml:"level"`
	// InitialDelay is the time to delay (duration) the scale down/up of this resource. If not specified its default value will be 30s.
	InitialDelay *time.Duration `yaml:"initialDelay,omitempty"`
	// ScaleTimeout is the time timeout duration to wait for when attempting to update the scaling sub-resource.
	Timeout *time.Duration `yaml:"timeout,omitempty"`
	// Replicas is the desired set of replicas. In case of scale down it represents the replicas to which it should scale down. If not specified its default value will be 0.
	// In case of a scale up it represents the replicas to which it should scale up to. If not specified its default value will be 1.
	Replicas *int32 `yaml:"replicas,omitempty"`
}
