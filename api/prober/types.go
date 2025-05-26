// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prober

import (
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MeltdownProtectionActive annotation captures if the DWD is active on a resource so that shoot reconciliation can ignore this resource from scaling up.
	MeltdownProtectionActive = "dependency-watchdog.gardener.cloud/meltdown-protection-active"
)

// Config provides typed access to prober configuration
type Config struct {
	// KubeConfigSecretName is the name of the kubernetes secret which has the kubeconfig to connect to the shoot control plane API server via internal domain
	KubeConfigSecretName string `json:"kubeConfigSecretName"`
	// ProbeInterval is the interval with which the probe will be run
	ProbeInterval *metav1.Duration `json:"probeInterval,omitempty"`
	// InitialDelay is the initial delay in running a probe for the first time
	InitialDelay *metav1.Duration `json:"initialDelay,omitempty"`
	// ProbeTimeout is the timeout that is set on the client which is used to reach the shoot control plane API server
	ProbeTimeout *metav1.Duration `json:"probeTimeout,omitempty"`
	// BackoffJitterFactor is the jitter with which a probe is run
	BackoffJitterFactor *float64 `json:"backoffJitterFactor,omitempty"`
	// DependentResourceInfos are the dependent resources that should be considered for scaling in case the shoot control API server cannot be reached via external domain
	DependentResourceInfos []DependentResourceInfo `json:"dependentResourceInfos"`
	// KCMNodeMonitorGraceDuration is the node-monitor-grace-period set in the kcm flags.
	KCMNodeMonitorGraceDuration *metav1.Duration `json:"kcmNodeMonitorGraceDuration,omitempty"`
	// NodeLeaseFailureFraction is used to determine the maximum number of leases that can be expired for a lease probe to succeed.
	NodeLeaseFailureFraction *float64 `json:"nodeLeaseFailureFraction,omitempty"`
}

// DependentResourceInfo captures a dependent resource which should be scaled
type DependentResourceInfo struct {
	// Ref identifies a resource
	Ref *autoscalingv1.CrossVersionObjectReference `json:"ref"`
	// Optional should be false if this resource should be present. If the resource is optional then it should be true
	// If this field is not specified, then its zero value (false for boolean) will be assumed.
	Optional bool `json:"optional"`
	// ScaleUpInfo captures the configuration to scale up the resource identified by Ref
	ScaleUpInfo *ScaleInfo `json:"scaleUp,omitempty"`
	// ScaleDownInfo captures the configuration to scale down the resource identified by Ref
	ScaleDownInfo *ScaleInfo `json:"scaleDown,omitempty"`
}

// ScaleInfo captures the configuration required to scale a dependent resource
type ScaleInfo struct {
	// Level is used to order the dependent resources. Highest level or the first level starts at 0 and increments. Each dependent resource on a level will have to wait for
	// all resource in a previous level to finish their scaling operation. If there are more than one resource defined with the same level then they will be scaled concurrently.
	Level int `json:"level"`
	// InitialDelay is the time to delay (duration) the scale down/up of this resource. If not specified its default value will be 0s.
	InitialDelay *metav1.Duration `json:"initialDelay,omitempty"`
	// ScaleTimeout is the time timeout duration to wait for when attempting to update the scaling sub-resource.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}
