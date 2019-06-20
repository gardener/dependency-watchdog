// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package restarter

import (
	"io/ioutil"
	"time"

	"github.com/ghodss/yaml"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoadServiceDependants creates the ServiceDependants from a config-file.
func LoadServiceDependants(file string) (*ServiceDependants, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return DecodeConfigFile(data)
}

// DecodeConfigFile decodes the byte stream to ServiceDependants objects.
func DecodeConfigFile(data []byte) (*ServiceDependants, error) {
	dependants := new(ServiceDependants)
	err := yaml.Unmarshal(data, dependants)
	if err != nil {
		return nil, err
	}
	return dependants, nil
}

// IsPodAvailable returns true if a pod is available; false otherwise.
// Precondition for an available pod is that it must be ready. On top
// of that, there are two cases when a pod can be considered available:
// 1. minReadySeconds == 0, or
// 2. LastTransitionTime (is set) + minReadySeconds < current time
func IsPodAvailable(pod *v1.Pod, minReadySeconds int32, now metav1.Time) bool {
	if !IsPodReady(pod) {
		return false
	}

	c := GetPodReadyCondition(pod.Status)
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	if minReadySeconds == 0 || !c.LastTransitionTime.IsZero() && c.LastTransitionTime.Add(minReadySecondsDuration).Before(now.Time) {
		return true
	}
	return false
}

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodDeleted returns true if a pod is deleted; false otherwise.
func IsPodDeleted(pod *v1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// GetPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)
	return condition
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

// ShouldDeletePod checks if the pod is in CrashloopBackoff and decides to delete the pod if its is
// not already deleted.
func ShouldDeletePod(pod *v1.Pod) bool {
	return !IsPodDeleted(pod) && IsPodInCrashloopBackoff(pod.Status)
}

// IsPodInCrashloopBackoff checks if the pod is in CrashloopBackoff from its status fields.
func IsPodInCrashloopBackoff(status v1.PodStatus) bool {
	for _, containerStatus := range status.ContainerStatuses {
		if isContainerInCrashLoopBackOff(containerStatus.State) {
			return true
		}
	}
	return false
}

func isContainerInCrashLoopBackOff(containerState v1.ContainerState) bool {
	if containerState.Waiting != nil {
		return containerState.Waiting.Reason == crashLoopBackOff
	}
	return false
}

// IsReadyEndpointPresentInSubsets checks if the endpoint resource have a subset of ready
// IP endpoints.
func IsReadyEndpointPresentInSubsets(subsets []v1.EndpointSubset) bool {
	for _, subset := range subsets {
		if len(subset.Addresses) != 0 {
			return true
		}
	}
	return false
}
