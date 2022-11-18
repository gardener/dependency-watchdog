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

package scaler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
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

func createTestDeploymentDependentResourceInfo(name string, scaleUpLevel, scaleDownLevel int, timeout *time.Duration, initialDelay *time.Duration, optional bool) papi.DependentResourceInfo {
	if timeout == nil {
		timeout = pointer.Duration(defaultTimeout)
	}
	if initialDelay == nil {
		initialDelay = pointer.Duration(defaultInitialDelay)
	}
	return papi.DependentResourceInfo{
		Ref:      &autoscalingv1.CrossVersionObjectReference{Name: name, Kind: deploymentKind, APIVersion: deploymentAPIVersion},
		Optional: &optional,
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
		for i := 0; i < v; i++ {
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
	levelStartIndex := strings.Index(levelStr, "-")
	if levelStartIndex < 0 {
		return 0, nil, fmt.Errorf("taskID should be of the format scale:level-<level>:<# separated list of resourceRefName>, given %s does not match this format", taskID)
	}
	level, err := strconv.Atoi(levelStr[levelStartIndex+1:])
	if err != nil {
		return 0, nil, err
	}
	// resNamesSplits the resource reference names
	resNamesSplits := strings.Split(taskIDSplits[2], "#")

	return level, resNamesSplits, nil
}
