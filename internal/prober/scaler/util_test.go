package scaler

import (
	"testing"

	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"

	papi "github.com/gardener/dependency-watchdog/api/prober"
)

func TestCreateScalableResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []papi.DependentResourceInfo
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 1, 0, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, nil, true))

	resInfos := createScalableResourceInfos(scaleUp, depResInfos)
	g.Expect(resInfos).To(HaveLen(len(depResInfos)))
	expectedObjectRefs := []autoscalingv1.CrossVersionObjectReference{mcmObjectRef, kcmObjectRef, caObjectRef}
	for _, resInfo := range resInfos {
		g.Expect(expectedObjectRefs).Should(ContainElement(Equal(*resInfo.ref)))
	}
}

func TestSortAndGetUniqueLevels(t *testing.T) {
	g := NewWithT(t)
	table := []struct {
		numLevel0 int
		numLevel1 int
		numLevel2 int
	}{
		{1, 2, 0},
		{1, 1, 1},
		{3, 0, 0},
		{0, 1, 2},
	}
	for _, entry := range table {
		resInfos := createTestScalableResourceInfos(map[int]int{1: entry.numLevel1, 0: entry.numLevel0, 2: entry.numLevel2})
		uniqueLevels := sortAndGetUniqueLevels(resInfos)
		var expectedOrderedLevels []int
		for i, numInLevel := range []int{entry.numLevel0, entry.numLevel1, entry.numLevel2} {
			if numInLevel > 0 {
				expectedOrderedLevels = append(expectedOrderedLevels, i)
			}
		}
		g.Expect(uniqueLevels).To(HaveLen(len(expectedOrderedLevels)))
		g.Expect(uniqueLevels).To(Equal(expectedOrderedLevels))
	}
}

func TestCollectResourceInfosByLevel(t *testing.T) {

}
