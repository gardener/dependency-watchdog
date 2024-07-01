// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package prober

import (
	"context"
	"testing"

	papi "github.com/gardener/dependency-watchdog/api/prober"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
)

const proberMgrTestNamespace = "default"

var pmLogger = logr.Discard()

func setupMgrTest(t *testing.T) (Manager, func(mgr Manager)) {
	g := NewWithT(t)
	mgr := NewManager()
	g.Expect(mgr).ShouldNot(BeNil(), "NewManager should return a non nil manager")

	return mgr, func(mgr Manager) {
		for _, p := range mgr.GetAllProbers() {
			mgr.Unregister(p.namespace)
		}
	}
}

func TestRegisterNewProberAndCheckIfItExistsAndIsNotClosed(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	p := NewProber(context.Background(), nil, proberMgrTestNamespace, &papi.Config{}, nil, nil, nil, pmLogger)
	g.Expect(p).ShouldNot(BeNil(), "NewProber should have returned a non nil Prober")
	g.Expect(p.namespace).Should(Equal(proberMgrTestNamespace), "The namespace of the created prober should match")
	g.Expect(mgr.Register(*p)).To(BeTrue(), "mgr.Register should register a new prober")

	foundProber, ok := mgr.GetProber(proberMgrTestNamespace)
	g.Expect(ok).Should(BeTrue(), "mgr.GetProber should return true for a registered prober")
	g.Expect(foundProber).ShouldNot(BeNil(), "mgr.GetProber should get the registered prober")
	g.Expect(foundProber.namespace).Should(Equal(proberMgrTestNamespace))
	g.Expect(foundProber.IsClosed()).Should(BeFalse(), "mgr.GetProber should not cancel the prober")

	t.Log("New prober registered and is not closed")
}

func TestProberRegistrationWithSameKeyShouldNotOverwriteExistingProber(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	p1 := NewProber(context.Background(), nil, proberMgrTestNamespace, &papi.Config{KubeConfigSecretName: "bingo"}, nil, nil, nil, pmLogger)
	g.Expect(mgr.Register(*p1)).To(BeTrue(), "mgr.Register should register a new prober")

	p2 := NewProber(context.Background(), nil, proberMgrTestNamespace, &papi.Config{KubeConfigSecretName: "zingo"}, nil, nil, nil, pmLogger)
	g.Expect(mgr.Register(*p2)).To(BeFalse(), "mgr.Register should return false if a prober with the same key is already registered")

	foundProber, ok := mgr.GetProber(proberMgrTestNamespace)
	g.Expect(ok).Should(BeTrue(), "mgr.Register should not remove the existing prober")
	g.Expect(foundProber.config.KubeConfigSecretName).ShouldNot(Equal(p2.config.KubeConfigSecretName), "mgr.Register should not replace the existing prober with a new one")
	g.Expect(foundProber.config.KubeConfigSecretName).Should(Equal(p1.config.KubeConfigSecretName))

	t.Log("Existing prober is not overwritten by the Register method")
}

func TestUnregisterExistingProberShouldCloseItAndRemoveItFromManager(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	p := NewProber(context.Background(), nil, proberMgrTestNamespace, &papi.Config{}, nil, nil, nil, pmLogger)
	g.Expect(mgr.Register(*p)).To(BeTrue(), "mgr.Register should register a new prober")

	mgr.Unregister(proberMgrTestNamespace)
	_, ok := mgr.GetProber(proberMgrTestNamespace)
	g.Expect(ok).Should(BeFalse(), "mgr.Unregister should delete the prober for the corresponding key")
	g.Eventually(p.IsClosed).Should(BeTrue(), "mgr.Unregister should cancel the unregistered prober")

	t.Log("De-registered existing prober and closed it")

}

func TestUnregisterNonExistingProberShouldNotFail(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	g.Expect(mgr.Unregister("bazingo")).To(BeFalse(), "mgr.Unregister should return false for non existing prober")
	t.Log("De-registering a non existing prober did not fail")

}
