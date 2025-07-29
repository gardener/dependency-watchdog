// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package weeder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// This label is used to match the EndpointSlice with the service it belongs to.
	// It is used in the predicate MatchingEndpoints to filter out EndpointSlices that are not relevant for weeding.
	// It is also used in the weeder to identify the service for which the weeder is created.
	ServiceNameLabel = "kubernetes.io/service-name"
)

// Config provides typed access weeder configuration
type Config struct {
	// WatchDuration is the duration for which all dependent pods for a service under surveillance will be watched after the service has recovered.
	// If the dependent pods have not transitioned to CrashLoopBackOff in this duration then it is assumed that they will not enter that state.
	WatchDuration *metav1.Duration `json:"watchDuration,omitempty"`
	// ServicesAndDependantSelectors is a map whose key is the service name and the value is a DependantSelectors
	ServicesAndDependantSelectors map[string]DependantSelectors `json:"servicesAndDependantSelectors"`
}

// DependantSelectors encapsulates LabelSelector's used to identify dependants for a service.
// [Trivia]: Dependent is used as an adjective and dependant is used as a noun. This explains the choice of the variant.
type DependantSelectors struct {
	// PodSelectors is a slice of LabelSelector's used to identify dependant pods
	PodSelectors []*metav1.LabelSelector `json:"podSelectors"`
}
