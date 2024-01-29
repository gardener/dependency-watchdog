// Copyright 2023 SAP SE or an SAP affiliate company
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

package cluster

import (
	"encoding/json"
	"testing"

	"github.com/gardener/dependency-watchdog/internal/test"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestCreateAndDeletePredicateFunc(t *testing.T) {
	tests := []struct {
		title          string
		run            func(g *WithT, numWorkers int) bool
		numWorkers     int
		expectedResult bool
	}{
		{"test: create predicate func for cluster having workers", testCreatePredicateFunc, 2, true},
		{"test: create predicate func for cluster having no workers", testCreatePredicateFunc, 0, false},
		{"test: delete predicate func for cluster having workers", testDeletePredicateFunc, 2, true},
		{"test: delete predicate func for cluster having no workers", testDeletePredicateFunc, 0, false},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			g := NewWithT(t)
			predicateResult := test.run(g, test.numWorkers)
			g.Expect(predicateResult).To(Equal(test.expectedResult))
		})
	}
}

func testCreatePredicateFunc(g *WithT, numWorkers int) bool {
	cluster, _, err := test.CreateClusterResource(numWorkers, nil, true)
	g.Expect(err).ToNot(HaveOccurred())
	e := event.CreateEvent{Object: cluster}
	predicateFuncs := workerLessShoot(logr.Discard())
	return predicateFuncs.Create(e)
}

func testDeletePredicateFunc(g *WithT, numWorkers int) bool {
	cluster, _, err := test.CreateClusterResource(numWorkers, nil, true)
	g.Expect(err).ToNot(HaveOccurred())
	e := event.DeleteEvent{Object: cluster}
	predicateFuncs := workerLessShoot(logr.Discard())
	return predicateFuncs.Delete(e)
}

func TestUpdatePredicateFunc(t *testing.T) {
	tests := []struct {
		title          string
		oldNumWorkers  int
		newNumWorkers  int
		expectedResult bool
	}{
		{"test: both old and new cluster do not have workers", 0, 0, false},
		{"test: old cluster has no workers and new cluster has workers", 0, 1, true},
		{"test: old cluster has workers and new cluster do not have workers", 2, 0, true},
		{"test: both old and new cluster have workers and there is no change", 1, 1, true},
		{"test: both old and new cluster have workers and are different in number", 1, 2, true},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			g := NewWithT(t)
			predicateResult := testUpdatePredicateFunc(g, test.oldNumWorkers, test.newNumWorkers)
			g.Expect(predicateResult).To(Equal(test.expectedResult))
		})
	}
}

func testUpdatePredicateFunc(g *WithT, oldNumWorker, newNumWorkers int) bool {
	oldCluster, _, err := test.CreateClusterResource(oldNumWorker, nil, true)
	g.Expect(err).ToNot(HaveOccurred())
	newCluster, _, err := test.CreateClusterResource(newNumWorkers, nil, true)
	g.Expect(err).ToNot(HaveOccurred())
	e := event.UpdateEvent{
		ObjectOld: oldCluster,
		ObjectNew: newCluster,
	}
	predicateFuncs := workerLessShoot(logr.Discard())
	return predicateFuncs.Update(e)
}

func TestGenericPredicateFunc(t *testing.T) {
	g := NewWithT(t)
	predicateFuncs := workerLessShoot(logr.Discard())
	e := event.GenericEvent{}
	g.Expect(predicateFuncs.Generic(e)).To(BeTrue())
}

func TestShootHasWorkersForNonShootResource(t *testing.T) {
	g := NewWithT(t)
	seed := gardencorev1beta1.Seed{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-aws",
		},
	}
	result := shootHasWorkers(&seed, logr.Discard())
	g.Expect(result).To(BeFalse())
}

func TestShootHasWorkersForInvalidShootResource(t *testing.T) {
	g := NewWithT(t)
	cluster, _, err := test.CreateClusterResource(0, nil, false)
	g.Expect(err).ToNot(HaveOccurred())
	seed := gardencorev1beta1.Seed{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-aws",
		},
	}
	seedBytes, err := json.Marshal(seed)
	g.Expect(err).ToNot(HaveOccurred())
	cluster.Spec.Shoot.Raw = seedBytes
	result := shootHasWorkers(cluster, logr.Discard())
	g.Expect(result).To(BeFalse())
}
