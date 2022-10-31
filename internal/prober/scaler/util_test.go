package scaler

import (
	"fmt"
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
		level0ResInfos := createTestScalableResourceInfos(map[int]int{0: entry.numLevel0})
		level1ResInfos := createTestScalableResourceInfos(map[int]int{1: entry.numLevel1})
		level2ResInfos := createTestScalableResourceInfos(map[int]int{2: entry.numLevel2})
		resInfos := make([]scalableResourceInfo, 0, entry.numLevel0+entry.numLevel1+entry.numLevel2)
		resInfos = append(resInfos, level0ResInfos...)
		resInfos = append(resInfos, level1ResInfos...)
		resInfos = append(resInfos, level2ResInfos...)

		resInfosByLevel := collectResourceInfosByLevel(resInfos)

		if entry.numLevel0 == 0 {
			_, ok := resInfosByLevel[0]
			g.Expect(ok).To(BeFalse())
		}
		g.Expect(resInfosByLevel[0]).To(HaveLen(entry.numLevel0))
		g.Expect(resInfosByLevel[0]).To(Equal(level0ResInfos))

		if entry.numLevel1 == 0 {
			_, ok := resInfosByLevel[1]
			g.Expect(ok).To(BeFalse())
		}
		g.Expect(resInfosByLevel[1]).To(HaveLen(entry.numLevel1))
		g.Expect(resInfosByLevel[1]).To(Equal(level1ResInfos))

		if entry.numLevel2 == 0 {
			_, ok := resInfosByLevel[2]
			g.Expect(ok).To(BeFalse())
		}
		g.Expect(resInfosByLevel[2]).To(HaveLen(entry.numLevel2))
		g.Expect(resInfosByLevel[2]).To(Equal(level2ResInfos))
	}
}

func TestMapToCrossVersionObjectRef(t *testing.T) {
	g := NewWithT(t)
	resInfos := createTestScalableResourceInfos(map[int]int{0: 1})
	objRefs := mapToCrossVersionObjectRef(resInfos)
	g.Expect(objRefs).To(HaveLen(1))
	g.Expect(objRefs[0]).To(Equal(*resInfos[0].ref))
}

func TestMapToCrossVersionObjectRefForEmptyResInfos(t *testing.T) {
	g := NewWithT(t)
	objRefs := mapToCrossVersionObjectRef([]scalableResourceInfo{})
	g.Expect(objRefs).To(HaveLen(0))
}

func TestCreateTaskName(t *testing.T) {
	g := NewWithT(t)
	level := 1
	resInfos := createTestScalableResourceInfos(map[int]int{level: 2})
	expectedTaskName := fmt.Sprintf("scale:level-%d:resource-10#resource-11", level)
	taskName := createTaskName(resInfos, level)
	g.Expect(taskName).To(Equal(expectedTaskName))
}
