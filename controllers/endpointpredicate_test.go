package controllers

import (
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"testing"
)

func turnReady(ep *v1.Endpoints) {
	nodeName := "node-0"
	ep.Subsets = []v1.EndpointSubset{
		{
			Addresses: []v1.EndpointAddress{
				{
					IP:       "10.1.0.52",
					NodeName: &nodeName,
				},
			},
			NotReadyAddresses: []v1.EndpointAddress{},
			Ports:             []v1.EndpointPort{},
		},
	}
}

func TestReadyEndpoints(t *testing.T) {
	g := NewWithT(t)
	predicate := ReadyEndpoints()

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
		t.Run(tc.name, func(t *testing.T) {
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
