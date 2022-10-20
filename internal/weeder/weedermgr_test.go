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

package weeder

import (
	"context"
	"testing"
	"time"

	v12 "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/gomega"
)

const (
	namespace = "hawai"
	epName    = "etcd-main"
)

var (
	testWatchDuration                 = 10 * time.Second
	testServicesAndDependantSelectors = map[string]v12.DependantSelectors{epName: {PodSelectors: []*metav1.LabelSelector{{MatchLabels: nil, MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "gardener.cloud/component", Operator: "In", Values: []string{"control-plane"}}}}}}}
	testWeederConfig                  = &v12.Config{
		WatchDuration:                 &metav1.Duration{Duration: testWatchDuration},
		ServicesAndDependantSelectors: testServicesAndDependantSelectors,
	}
	testEp = &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: epName},
	}
)

func setupMgrTest(t *testing.T) (Manager, func(mgr Manager)) {
	g := NewWithT(t)
	mgr := NewManager()
	g.Expect(mgr).ShouldNot(BeNil(), "NewManager should return a non nil manager")
	return mgr, func(mgr Manager) {
		g.Expect(mgr.UnregisterAll()).To(BeTrue())
	}
}

func TestRegisterNewWeederAndIsNotClosed(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	w := NewWeeder(context.Background(), namespace, testWeederConfig, nil, nil, testEp, logr.New(log.NullLogSink{}))
	g.Expect(w).ShouldNot(BeNil(), "NewWeeder should have returned a non nil weeder")
	g.Expect(mgr.Register(*w)).To(BeTrue(), "mgr.Register should register a new weeder")

	key := createKey(*w)
	foundWeederRegistration, ok := mgr.GetWeederRegistration(key)
	g.Expect(ok).Should(BeTrue(), "mgr.GetProber should return true for a registered weeder")
	g.Expect(foundWeederRegistration.IsClosed()).To(BeFalse(), "Registered weeder should be alive")

	t.Log("Registering a weeder succeeded")
}

func TestRegisterWeederWithSameKeyShouldReplaceOldEntry(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	w1 := NewWeeder(context.Background(), namespace, testWeederConfig, nil, nil, testEp, logr.New(log.NullLogSink{}))
	g.Expect(mgr.Register(*w1)).To(BeTrue(), "mgr.Register should register the first weeder")
	key := createKey(*w1)
	foundWeederRegistration1, _ := mgr.GetWeederRegistration(key)
	g.Expect(foundWeederRegistration1.IsClosed()).To(BeFalse(), "First Registered weeder should be alive")

	w2 := NewWeeder(context.Background(), namespace, testWeederConfig, nil, nil, testEp, logr.New(log.NullLogSink{}))
	g.Expect(mgr.Register(*w2)).To(BeTrue(), "mgr.Register should register the second weeder")
	foundWeederRegistration2, _ := mgr.GetWeederRegistration(key)

	g.Expect(foundWeederRegistration1.IsClosed()).To(BeTrue(), "First Registered weeder should be cancelled")
	g.Expect(foundWeederRegistration2.IsClosed()).To(BeFalse(), "Second Registered weeder should be alive")

	t.Log("Registering a weeder with same key successfully replaced and cancelled old weeder")
}

func TestUnregisterExistingWeederShouldCloseItAndRemoveItFromManager(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	w := NewWeeder(context.Background(), namespace, testWeederConfig, nil, nil, testEp, logr.New(log.NullLogSink{}))
	g.Expect(mgr.Register(*w)).To(BeTrue(), "mgr.Register should register the first weeder")
	key := createKey(*w)
	foundWeederRegistration, _ := mgr.GetWeederRegistration(key)
	g.Expect(foundWeederRegistration.IsClosed()).To(BeFalse(), "Registered weeder should be alive")

	g.Expect(mgr.Unregister(key)).To(BeTrue(), "mgr.Unregister should unregister the existing weeder")
	_, ok := mgr.GetWeederRegistration(key)
	g.Expect(ok).To(BeFalse(), "mgr.Unregister should delete the weeder for the corresponding key")
	g.Eventually(foundWeederRegistration.IsClosed()).Should(BeTrue(), "mgr.Unregister should cancel the unregistered weeder")

	t.Log("De-registering a existing weeder succeeded")
}

func TestUnregisterNonExistingWeederShouldNotFail(t *testing.T) {
	g := NewWithT(t)
	mgr, tearDownTest := setupMgrTest(t)
	defer tearDownTest(mgr)

	g.Expect(mgr.Unregister("random-key")).To(BeFalse(), "mgr.Unregister should return false for non existing weeder")
	t.Log("De-registering a non-existing weeder did not fail")
}
