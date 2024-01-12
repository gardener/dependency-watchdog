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

//go:build !kind_tests

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
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, nil, nil, false))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 1, 0, nil, nil, false))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, nil, nil, false))

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
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 1, 0, pointer.Duration(timeout), pointer.Duration(initialDelay), false))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 1, 0, pointer.Duration(timeout), pointer.Duration(initialDelay), false))
	depResInfos = append(depResInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 0, 1, pointer.Duration(timeout), pointer.Duration(initialDelay), false))

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
	g.Expect(objRefs).To(BeEmpty())
}

func TestCreateTaskName(t *testing.T) {
	g := NewWithT(t)
	level := 1
	resInfos := createTestScalableResourceInfos(map[int]int{level: 2})
	expectedTaskName := fmt.Sprintf("scale:level-%d:resource-10#resource-11", level)
	taskName := createTaskName(resInfos, level)
	g.Expect(taskName).To(Equal(expectedTaskName))
}
