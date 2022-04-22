package prober

import (
	"fmt"
	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"reflect"
	"testing"
	"time"
)

var (
	defaultInitialDelay = 10 * time.Second
	defaultTimeout      = 20 * time.Second
	mcmRef              = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "machine-controller-manager", APIVersion: "apps/v1"}
	kcmRef              = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "kube-controller-manager", APIVersion: "apps/v1"}
	caRef               = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "cluster-autoscaler", APIVersion: "apps/v1"}
)

func TestSortAndGetUniqueLevels(t *testing.T) {
	g := NewWithT(t)
	numResInfosByLevel := map[int]int{2: 1, 0: 2, 1: 2}
	resInfos := createScaleableResourceInfos(numResInfosByLevel)
	levels := sortAndGetUniqueLevels(resInfos)
	g.Expect(levels).ToNot(BeNil())
	g.Expect(levels).ToNot(BeEmpty())
	g.Expect(len(levels)).To(Equal(3))
	g.Expect(levels).To(Equal([]int{0, 1, 2}))
}

func TestSortAndGetUniqueLevelsForEmptyScaleableResourceInfos(t *testing.T) {
	g := NewWithT(t)
	levels := sortAndGetUniqueLevels([]scaleableResourceInfo{})
	g.Expect(levels).To(BeNil())
}

func TestCreateScaleUpResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []DependentResourceInfo
	depResInfos = append(depResInfos, createDependentResourceInfo(mcmRef.Name, 2, 0, 1, 0))
	depResInfos = append(depResInfos, createDependentResourceInfo(caRef.Name, 0, 1, 1, 0))
	depResInfos = append(depResInfos, createDependentResourceInfo(kcmRef.Name, 1, 0, 1, 0))

	scaleUpResInfos := createScaleUpResourceInfos(depResInfos)
	g.Expect(scaleUpResInfos).ToNot(BeNil())
	g.Expect(scaleUpResInfos).ToNot(BeEmpty())
	g.Expect(len(scaleUpResInfos)).To(Equal(len(depResInfos)))

	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: mcmRef, level: 2, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: caRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: kcmRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
}

func TestCreateScaleDownResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []DependentResourceInfo
	depResInfos = append(depResInfos, createDependentResourceInfo(mcmRef.Name, 1, 0, 1, 0))
	depResInfos = append(depResInfos, createDependentResourceInfo(caRef.Name, 0, 1, 2, 1))
	depResInfos = append(depResInfos, createDependentResourceInfo(kcmRef.Name, 1, 0, 1, 0))

	scaleDownResInfos := createScaleDownResourceInfos(depResInfos)
	g.Expect(scaleDownResInfos).ToNot(BeNil())
	g.Expect(scaleDownResInfos).ToNot(BeEmpty())
	g.Expect(len(scaleDownResInfos)).To(Equal(len(depResInfos)))

	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: mcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0}, scaleDownResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: caRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleDownResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: kcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0}, scaleDownResInfos)).To(BeTrue())
}

// utility methods to be used by tests
//------------------------------------------------------------------------------------------------------------------
// createScaleableResourceInfos creates a slice of scaleableResourceInfo's taking in a map whose key is level
// and value is the number of scaleableResourceInfo's to be created at that level
func createScaleableResourceInfos(numResInfosByLevel map[int]int) []scaleableResourceInfo {
	var resInfos []scaleableResourceInfo
	for k, v := range numResInfosByLevel {
		for i := 0; i < v; i++ {
			resInfos = append(resInfos, scaleableResourceInfo{
				ref:   autoscalingv1.CrossVersionObjectReference{Name: fmt.Sprintf("resource-%d%d", k, i)},
				level: k,
			})
		}
	}
	return resInfos
}

func createDependentResourceInfo(name string, scaleUpLevel, scaleDownLevel int, scaleUpReplicas, scaleDownReplicas int32) DependentResourceInfo {
	return DependentResourceInfo{
		Ref: autoscalingv1.CrossVersionObjectReference{Name: name, Kind: "Deployment", APIVersion: "apps/v1"},
		ScaleUpInfo: &ScaleInfo{
			Level:        scaleUpLevel,
			InitialDelay: &defaultInitialDelay,
			Timeout:      &defaultTimeout,
			Replicas:     &scaleUpReplicas,
		},
		ScaleDownInfo: &ScaleInfo{
			Level:        scaleDownLevel,
			InitialDelay: &defaultInitialDelay,
			Timeout:      &defaultTimeout,
			Replicas:     &scaleDownReplicas,
		},
	}
}

func scaleableResourceMatchFound(expected scaleableResourceInfo, resources []scaleableResourceInfo) bool {
	for _, resInfo := range resources {
		if resInfo.ref.Name == expected.ref.Name {
			// compare all values which are not nil
			return reflect.DeepEqual(expected.ref, resInfo.ref) && expected.level == resInfo.level && expected.replicas == resInfo.replicas
		}
	}
	return false
}
