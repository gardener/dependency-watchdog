// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package endpoint

import (
	"testing"

	"k8s.io/utils/ptr"

	v12 "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestReadyEndpoints(t *testing.T) {
	g := NewWithT(t)
	predicate := ReadyEndpoints(logr.Discard())

	readyEpSlice := &discoveryv1.EndpointSlice{}
	turnReady(readyEpSlice)

	notReadyEpSlice := &discoveryv1.EndpointSlice{}
	turnNotReady(notReadyEpSlice)

	testcases := []struct {
		name                             string
		ep                               *discoveryv1.EndpointSlice
		oldEp                            *discoveryv1.EndpointSlice
		expectedCreateEventFilterOutput  bool
		expectedUpdateEventFilterOutput  bool
		expectedDeleteEventFilterOutput  bool
		expectedGenericEventFilterOutput bool
	}{
		{
			name:                             "no ep -> Ready ep",
			ep:                               readyEpSlice,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "no ep -> NotReady ep",
			ep:                               notReadyEpSlice,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "NotReady ep -> Ready ep",
			ep:                               readyEpSlice,
			oldEp:                            notReadyEpSlice,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  true,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Ready ep -> Ready ep",
			ep:                               readyEpSlice,
			oldEp:                            readyEpSlice,
			expectedCreateEventFilterOutput:  true,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: true,
		},
		{
			name:                             "Ready ep -> no ep",
			oldEp:                            readyEpSlice,
			expectedCreateEventFilterOutput:  false,
			expectedUpdateEventFilterOutput:  false,
			expectedDeleteEventFilterOutput:  false,
			expectedGenericEventFilterOutput: false,
		},
		{
			name:                             "NotReady ep -> no ep",
			oldEp:                            notReadyEpSlice,
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

	epRelevant := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ep-relevant",
			Labels: map[string]string{v12.ServiceNameLabel: "ep-relevant"},
		},
	}

	epIrrelevant := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ep-irrelevant",
			Labels: map[string]string{v12.ServiceNameLabel: "ep-irrelevant"},
		},
	}

	testcases := []struct {
		name                             string
		ep                               *discoveryv1.EndpointSlice
		oldEp                            *discoveryv1.EndpointSlice
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

func turnReady(epSlice *discoveryv1.EndpointSlice) {
	epSlice.Endpoints = []discoveryv1.Endpoint{
		{
			Addresses: []string{"10.1.0.52"},
			NodeName:  ptr.To("node-0"),
			Conditions: discoveryv1.EndpointConditions{
				Ready: ptr.To(true),
			},
		},
	}
	epSlice.Ports = []discoveryv1.EndpointPort{}
}

func turnNotReady(epSlice *discoveryv1.EndpointSlice) {
	epSlice.Endpoints = []discoveryv1.Endpoint{
		{
			Addresses: []string{"10.1.0.52"},
			NodeName:  ptr.To("node-0"),
			Conditions: discoveryv1.EndpointConditions{
				Ready: ptr.To(false),
			},
		},
	}
	epSlice.Ports = []discoveryv1.EndpointPort{}
}
