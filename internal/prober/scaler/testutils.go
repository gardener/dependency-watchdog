// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/utils/ptr"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultTimeout       = 10 * time.Second
	defaultInitialDelay  = 10 * time.Millisecond
	deploymentKind       = "Deployment"
	deploymentAPIVersion = "apps/v1"
)

var (
	mcmObjectRef = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "machine-controller-manager", APIVersion: "apps/v1"}
	kcmObjectRef = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "kube-controller-manager", APIVersion: "apps/v1"}
	caObjectRef  = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "cluster-autoscaler", APIVersion: "apps/v1"}
)

// nolint:unparam
func createTestDeploymentDependentResourceInfo(name string, scaleUpLevel, scaleDownLevel int, timeout *time.Duration, initialDelay *time.Duration, optional bool) papi.DependentResourceInfo {
	if timeout == nil {
		timeout = ptr.To(defaultTimeout)
	}
	if initialDelay == nil {
		initialDelay = ptr.To(defaultInitialDelay)
	}
	return papi.DependentResourceInfo{
		Ref:      &autoscalingv1.CrossVersionObjectReference{Name: name, Kind: deploymentKind, APIVersion: deploymentAPIVersion},
		Optional: optional,
		ScaleUpInfo: &papi.ScaleInfo{
			Level:        scaleUpLevel,
			InitialDelay: &metav1.Duration{Duration: *initialDelay},
			Timeout:      &metav1.Duration{Duration: *timeout},
		},
		ScaleDownInfo: &papi.ScaleInfo{
			Level:        scaleDownLevel,
			InitialDelay: &metav1.Duration{Duration: *initialDelay},
			Timeout:      &metav1.Duration{Duration: *timeout},
		},
	}
}

func createTestScalableResourceInfos(numResInfosByLevel map[int]int) []scalableResourceInfo {
	var resInfos []scalableResourceInfo
	for k, v := range numResInfosByLevel {
		for i := range v {
			resInfos = append(resInfos, scalableResourceInfo{
				ref:   &autoscalingv1.CrossVersionObjectReference{Name: fmt.Sprintf("resource-%d%d", k, i)},
				level: k,
			})
		}
	}
	return resInfos
}

func parseTaskID(taskID string) (int, []string, error) {
	taskIDSplits := strings.Split(taskID, ":")
	if len(taskIDSplits) < 3 {
		return 0, nil, fmt.Errorf("taskID should be of the format scale:level-<level>:<# separated list of resourceRefName>, given %s does not match this format", taskID)
	}
	levelStr := taskIDSplits[1]
	_, after, ok := strings.Cut(levelStr, "-")
	if !ok {
		return 0, nil, fmt.Errorf("taskID should be of the format scale:level-<level>:<# separated list of resourceRefName>, given %s does not match this format", taskID)
	}
	level, err := strconv.Atoi(after)
	if err != nil {
		return 0, nil, err
	}
	// resNamesSplits the resource reference names
	resNamesSplits := strings.Split(taskIDSplits[2], "#")

	return level, resNamesSplits, nil
}
