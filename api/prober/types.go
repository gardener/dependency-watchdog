package prober

import (
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

// Config provides typed access to prober configuration
type Config struct {
	// InternalKubeConfigSecretName is the name of the kubernetes secret which has the kubeconfig to connect to the shoot control plane API server via internal domain
	InternalKubeConfigSecretName string `yaml:"internalKubeConfigSecretName"`
	// ExternalKubeConfigSecretName is the name of the kubernetes secret which has the kubeconfig to connect to the shoot control plane API server via external domain
	ExternalKubeConfigSecretName string `yaml:"externalKubeConfigSecretName"`
	// ProbeInterval is the interval with which the probe will be run
	ProbeInterval *time.Duration `yaml:"probeInterval,omitempty"`
	// InitialDelay is the initial delay in running a probe for the first time
	InitialDelay *time.Duration `yaml:"initialDelay,omitempty"`
	// ProbeTimeout is the timeout that is set on the client which is used to reach the shoot control plane API server
	ProbeTimeout *time.Duration `yaml:"probeTimeout,omitempty"`
	// SuccessThreshold is the number of consecutive times a probe is successful to ascertain that the probe is healthy
	SuccessThreshold *int `yaml:"successThreshold,omitempty"`
	// FailureThreshold is the number of consecutive times a probe is unsuccessful to ascertain that the probe is unhealthy
	FailureThreshold *int `yaml:"failureThreshold,omitempty"`
	// InternalProbeFailureBackoffDuration backoff duration if the internal probe is unhealthy, before attempting again
	InternalProbeFailureBackoffDuration *time.Duration `yaml:"internalProbeFailureBackoffDuration,omitempty"`
	// BackoffJitterFactor jitter with which a probe is run
	BackoffJitterFactor *float64 `yaml:"backoffJitterFactor,omitempty"`
	// DependentResourceInfos dependent resources that should be considered for scaling in case the shoot control API server cannot be reached via external domain
	DependentResourceInfos []DependentResourceInfo `yaml:"dependentResourceInfos"`
}

// DependentResourceInfo captures a dependent resource which should be scaled
type DependentResourceInfo struct {
	// Ref identifies a resource
	Ref *autoscalingv1.CrossVersionObjectReference `yaml:"ref"`
	// ShouldExist should be true if this resource should be present. If the resource is optional then it should be false.
	ShouldExist *bool `yaml:"shouldExist"`
	// ScaleUpInfo captures the configuration to scale up the resource identified by Ref
	ScaleUpInfo *ScaleInfo `yaml:"scaleUp,omitempty"`
	// ScaleDownInfo captures the configuration to scale down the resource identified by Ref
	ScaleDownInfo *ScaleInfo `yaml:"scaleDown,omitempty"`
}

// ScaleInfo captures the configuration required to scale a dependent resource
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
