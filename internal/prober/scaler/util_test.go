package scaler

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/utils/pointer"

	papi "github.com/gardener/dependency-watchdog/api/prober"
)

func TestCreateScaleUpResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []papi.DependentResourceInfo
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 1, 0, nil, nil, true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, nil, nil, true))

	resInfos := createScalableResourceInfos(scaleUp, depResInfos)
	g.Expect(resInfos).To(HaveLen(len(depResInfos)))
	expectedObjectRefs := []autoscalingv1.CrossVersionObjectReference{mcmObjectRef, caObjectRef, kcmObjectRef}
	for i, resInfo := range resInfos {
		g.Expect(expectedObjectRefs[i]).To(Equal(*resInfo.ref))
		g.Expect(depResInfos[i].ScaleUpInfo.Level).To(Equal(resInfo.level))
		g.Expect(resInfo.initialDelay).To(Equal(defaultInitialDelay))
		g.Expect(resInfo.timeout).To(Equal(defaultTimeout))
	}
}

func TestCreateScaleDownResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []papi.DependentResourceInfo
	const (
		timeout      = 20 * time.Second
		initialDelay = 45 * time.Second
	)
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, pointer.Duration(timeout), pointer.Duration(initialDelay), true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 1, 0, pointer.Duration(timeout), pointer.Duration(initialDelay), true))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, pointer.Duration(timeout), pointer.Duration(initialDelay), true))

	resInfos := createScalableResourceInfos(scaleDown, depResInfos)
	g.Expect(resInfos).To(HaveLen(len(depResInfos)))
	expectedObjectRefs := []autoscalingv1.CrossVersionObjectReference{mcmObjectRef, caObjectRef, kcmObjectRef}
	for i, resInfo := range resInfos {
		g.Expect(expectedObjectRefs[i]).To(Equal(*resInfo.ref))
		g.Expect(depResInfos[i].ScaleDownInfo.Level).To(Equal(resInfo.level))
		g.Expect(resInfo.initialDelay).To(Equal(initialDelay))
		g.Expect(resInfo.timeout).To(Equal(timeout))
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

func TestScaleUpReplicaMismatch(t *testing.T) {
	g := NewWithT(t)
	g.Expect(scaleUpReplicasPredicate(0)).To(BeTrue())
	g.Expect(scaleUpReplicasPredicate(2)).To(BeFalse())
	g.Expect(scaleUpReplicasPredicate(1)).To(BeFalse())
}

func TestScaleDownReplicaMismatch(t *testing.T) {
	g := NewWithT(t)
	g.Expect(scaleDownReplicasPredicate(1)).To(BeTrue())
	g.Expect(scaleDownReplicasPredicate(2)).To(BeTrue())
	g.Expect(scaleDownReplicasPredicate(0)).To(BeFalse())
}

func TestScaleUpCompletePredicate(t *testing.T) {
	g := NewWithT(t)
	g.Expect(scaleUpCompletePredicate(2, 3)).To(BeFalse())
	g.Expect(scaleUpCompletePredicate(2, 2)).To(BeTrue())
	g.Expect(scaleUpCompletePredicate(2, 1)).To(BeTrue())
}

func TestScaleDownCompletePredicate(t *testing.T) {
	g := NewWithT(t)
	g.Expect(scaleDownCompletePredicate(2, 0)).To(BeFalse())
	g.Expect(scaleDownCompletePredicate(0, 0)).To(BeTrue())
	g.Expect(scaleDownCompletePredicate(1, 2)).To(BeTrue())
}
