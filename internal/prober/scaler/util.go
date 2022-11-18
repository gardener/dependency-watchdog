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
	"sort"
	"strings"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

// createScalableResourceInfos creates slice of scalableResourceInfo from an operation and slice of papi.DependentResourceInfo.
func createScalableResourceInfos(op operation, dependentResourceInfos []papi.DependentResourceInfo) []scalableResourceInfo {
	resourceInfos := make([]scalableResourceInfo, 0, len(dependentResourceInfos))
	for _, depResInfo := range dependentResourceInfos {
		var (
			level                 int
			initialDelay, timeout time.Duration
		)
		if op == scaleUp {
			level = depResInfo.ScaleUpInfo.Level
			initialDelay = depResInfo.ScaleUpInfo.InitialDelay.Duration
			timeout = depResInfo.ScaleUpInfo.Timeout.Duration
		} else {
			level = depResInfo.ScaleDownInfo.Level
			initialDelay = depResInfo.ScaleDownInfo.InitialDelay.Duration
			timeout = depResInfo.ScaleDownInfo.Timeout.Duration
		}
		resInfo := scalableResourceInfo{
			ref:          depResInfo.Ref,
			optional:     *depResInfo.Optional,
			level:        level,
			initialDelay: initialDelay,
			timeout:      timeout,
			operation:    op,
		}
		resourceInfos = append(resourceInfos, resInfo)
	}
	return resourceInfos
}

func sortAndGetUniqueLevels(resourceInfos []scalableResourceInfo) []int {
	var levels []int
	keys := make(map[int]bool)
	for _, resInfo := range resourceInfos {
		if _, found := keys[resInfo.level]; !found {
			keys[resInfo.level] = true
			levels = append(levels, resInfo.level)
		}
	}
	sort.Ints(levels)
	return levels
}

func collectResourceInfosByLevel(resourceInfos []scalableResourceInfo) map[int][]scalableResourceInfo {
	resInfosByLevel := make(map[int][]scalableResourceInfo)
	for _, resInfo := range resourceInfos {
		level := resInfo.level
		if _, ok := resInfosByLevel[level]; !ok {
			var levelResInfos []scalableResourceInfo
			levelResInfos = append(levelResInfos, resInfo)
			resInfosByLevel[level] = levelResInfos
		} else {
			resInfosByLevel[level] = append(resInfosByLevel[level], resInfo)
		}
	}
	return resInfosByLevel
}

func mapToCrossVersionObjectRef(resourceInfos []scalableResourceInfo) []autoscalingv1.CrossVersionObjectReference {
	refs := make([]autoscalingv1.CrossVersionObjectReference, 0, len(resourceInfos))
	for _, resInfo := range resourceInfos {
		refs = append(refs, *resInfo.ref)
	}
	return refs
}

func createTaskName(resInfos []scalableResourceInfo, level int) string {
	resNames := make([]string, 0, len(resInfos))
	for _, resInfo := range resInfos {
		resNames = append(resNames, resInfo.ref.Name)
	}
	return fmt.Sprintf("scale:level-%d:%s", level, strings.Join(resNames, "#"))
}
