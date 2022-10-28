package scaler

import (
	"fmt"
	"sort"
	"strings"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

// createScalableResourceInfos creates slice of scalableResourceInfo from an operation and slice of papi.DependentResourceInfo.
func createScalableResourceInfos(opType operationType, dependentResourceInfos []papi.DependentResourceInfo) []scalableResourceInfo {
	resourceInfos := make([]scalableResourceInfo, 0, len(dependentResourceInfos))
	for _, depResInfo := range dependentResourceInfos {
		resInfo := scalableResourceInfo{
			ref:          depResInfo.Ref,
			shouldExist:  *depResInfo.ShouldExist,
			level:        depResInfo.ScaleUpInfo.Level,
			initialDelay: depResInfo.ScaleUpInfo.InitialDelay.Duration,
			timeout:      depResInfo.ScaleUpInfo.Timeout.Duration,
			operation:    newScaleOperation(opType),
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
