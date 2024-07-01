// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

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
		run            func(g *WithT, workerNames []string) bool
		workerNames    []string
		expectedResult bool
	}{
		{"test: create predicate func for cluster having workers", testCreatePredicateFunc, []string{test.Worker1Name, test.Worker2Name}, true},
		{"test: create predicate func for cluster having no workers", testCreatePredicateFunc, nil, false},
		{"test: delete predicate func for cluster having workers", testDeletePredicateFunc, []string{test.Worker1Name, test.Worker2Name}, true},
		{"test: delete predicate func for cluster having no workers", testDeletePredicateFunc, nil, false},
	}

	for _, entry := range tests {
		t.Run(entry.title, func(t *testing.T) {
			g := NewWithT(t)
			predicateResult := entry.run(g, entry.workerNames)
			g.Expect(predicateResult).To(Equal(entry.expectedResult))
		})
	}
}

func testCreatePredicateFunc(g *WithT, workerNames []string) bool {
	cluster, _, err := test.NewClusterBuilder().WithWorkerNames(workerNames).WithRawShoot(true).Build()
	g.Expect(err).ToNot(HaveOccurred())
	e := event.CreateEvent{Object: cluster}
	predicateFuncs := workerLessShoot(logr.Discard())
	return predicateFuncs.Create(e)
}

func testDeletePredicateFunc(g *WithT, workerNames []string) bool {
	cluster, _, err := test.NewClusterBuilder().WithWorkerNames(workerNames).WithRawShoot(true).Build()
	g.Expect(err).ToNot(HaveOccurred())
	e := event.DeleteEvent{Object: cluster}
	predicateFuncs := workerLessShoot(logr.Discard())
	return predicateFuncs.Delete(e)
}

func TestUpdatePredicateFunc(t *testing.T) {
	tests := []struct {
		title          string
		oldWorkerNames []string
		newWorkerNames []string
		expectedResult bool
	}{
		{"test: both old and new cluster do not have workers", nil, nil, false},
		{"test: old cluster has no workers and new cluster has workers", nil, []string{test.Worker1Name}, true},
		{"test: old cluster has workers and new cluster do not have workers", []string{test.Worker1Name, test.Worker2Name}, nil, true},
		{"test: both old and new cluster have workers and there is no change", []string{test.Worker1Name}, []string{test.Worker1Name}, true},
		{"test: both old and new cluster have workers and are different in number", []string{test.Worker1Name}, []string{test.Worker1Name, test.Worker2Name}, true},
	}

	for _, entry := range tests {
		t.Run(entry.title, func(t *testing.T) {
			g := NewWithT(t)
			predicateResult := testUpdatePredicateFunc(g, entry.oldWorkerNames, entry.newWorkerNames)
			g.Expect(predicateResult).To(Equal(entry.expectedResult))
		})
	}
}

func testUpdatePredicateFunc(g *WithT, oldWorkerNames, newWorkerNames []string) bool {
	oldCluster, _, err := test.NewClusterBuilder().WithWorkerNames(oldWorkerNames).WithRawShoot(true).Build()
	g.Expect(err).ToNot(HaveOccurred())
	newCluster, _, err := test.NewClusterBuilder().WithWorkerNames(newWorkerNames).WithRawShoot(true).Build()
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
	cluster, _, err := test.NewClusterBuilder().WithRawShoot(true).Build()
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
