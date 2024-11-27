// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package endpoint

import (
	"testing"

	v12 "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func turnReady(ep *v1.Endpoints) {
	ep.Subsets = []v1.EndpointSubset{
		{
			Addresses: []v1.EndpointAddress{
				{
					IP:       "10.1.0.52",
					NodeName: pointer.String("node-0"),
				},
			},
			NotReadyAddresses: []v1.EndpointAddress{},
			Ports:             []v1.EndpointPort{},
		},
	}
}

func TestReadyEndpoints(t *testing.T) {
	g := NewWithT(t)
	predicate := ReadyEndpoints(logr.Discard())

	readyEp := &v1.Endpoints{}
	turnReady(readyEp)

	notReadyEp := &v1.Endpoints{}

	testcases := []struct {
		name                             string
		ep                               *v1.Endpoints
		oldEp                            *v1.Endpoints
		expectedCreateEventFilterOutput  bool
		expectedUpdateEventFilterOutput  bool
		expectedDeleteEventFilterOutput  bool
		expectedGenericEventFilterOutput bool
	}{
		{
			name:                             "no ep -> Ready ep",
			ep:                               readyEp,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "no ep -> NotReady ep",
			ep:                               notReadyEp,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "NotReady ep -> Ready ep",
			ep:                               readyEp,
			oldEp:                            notReadyEp,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Ready ep -> Ready ep",
			ep:                               readyEp,
			oldEp:                            readyEp,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Ready ep -> no ep",
			oldEp:                            readyEp,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "NotReady ep -> no ep",
			oldEp:                            notReadyEp,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(_ *testing.T) {
			createEv := event.CreateEvent{
				Object: tc.ep,
			}
			updateEv := event.UpdateEvent{
				ObjectOld: tc.oldEp,
				ObjectNew: tc.ep,
			}
			deleteEv := event.DeleteEvent{
				Object: tc.ep,
			}
			genericEv := event.GenericEvent{
				Object: tc.ep,
			}

			g.Expect(predicate.Create(createEv)).To(Equal(tc.expectedCreateEventFilterOutput))
			g.Expect(predicate.Update(updateEv)).To(Equal(tc.expectedUpdateEventFilterOutput))
			g.Expect(predicate.Delete(deleteEv)).To(Equal(tc.expectedDeleteEventFilterOutput))
			g.Expect(predicate.Generic(genericEv)).To(Equal(tc.expectedGenericEventFilterOutput))
		})
	}
}

func TestMatchingEndpointsPredicate(t *testing.T) {
	g := NewWithT(t)

	epMap := map[string]v12.DependantSelectors{
		"ep-relevant": {},
	}

	predicate := MatchingEndpoints(epMap)

	epRelevant := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ep-relevant",
		},
	}

	epIrrelevant := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ep-irrelevant",
		},
	}

	testcases := []struct {
		name                             string
		ep                               *v1.Endpoints
		oldEp                            *v1.Endpoints
		expectedCreateEventFilterOutput  bool
		expectedUpdateEventFilterOutput  bool
		expectedDeleteEventFilterOutput  bool
		expectedGenericEventFilterOutput bool
	}{
		{
			name:                             "no ep -> Relevant ep",
			ep:                               epRelevant,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "no ep -> Irrelevant ep",
			ep:                               epIrrelevant,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "Relevant ep -> Relevant ep",
			ep:                               epRelevant,
			oldEp:                            epRelevant,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Relevant ep -> Irrelevant ep",
			ep:                               epIrrelevant,
			oldEp:                            epRelevant,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "Irrelevant ep -> Relevant ep",
			ep:                               epRelevant,
			oldEp:                            epIrrelevant,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Irrelevant ep -> Irrelevant ep",
			ep:                               epIrrelevant,
			oldEp:                            epIrrelevant,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "Relevant ep -> no ep",
			oldEp:                            epRelevant,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "Irrelevant ep -> no ep",
			oldEp:                            epIrrelevant,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(_ *testing.T) {
			createEv := event.CreateEvent{
				Object: tc.ep,
			}
			updateEv := event.UpdateEvent{
				ObjectOld: tc.oldEp,
				ObjectNew: tc.ep,
			}
			deleteEv := event.DeleteEvent{
				Object: tc.ep,
			}
			genericEv := event.GenericEvent{
				Object: tc.ep,
			}

			g.Expect(predicate.Create(createEv)).To(Equal(tc.expectedCreateEventFilterOutput))
			g.Expect(predicate.Update(updateEv)).To(Equal(tc.expectedUpdateEventFilterOutput))
			g.Expect(predicate.Delete(deleteEv)).To(Equal(tc.expectedDeleteEventFilterOutput))
			g.Expect(predicate.Generic(genericEv)).To(Equal(tc.expectedGenericEventFilterOutput))
		})
	}
}
