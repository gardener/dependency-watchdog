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

package prober

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsHealthy(t *testing.T) {
	g := NewWithT(t)
	unhealthy := createProbeStatus(0, 0, nil, nil)
	healthy := createProbeStatus(4, 0, nil, nil)
	successThreshold := 3

	g.Expect(unhealthy.isHealthy(successThreshold)).To(BeFalse(), "unhealthy.isHealthy should have been false")
	g.Expect(healthy.isHealthy(successThreshold)).To(BeTrue(), "healthy.isHealthy should have been true")
	t.Log("IsHealthy passed")
}

func TestIsUnhealthy(t *testing.T) {
	g := NewWithT(t)
	unhealthy := createProbeStatus(0, 4, nil, nil)
	healthy := createProbeStatus(0, 2, nil, nil)
	failureThreshold := 3

	g.Expect(unhealthy.isUnhealthy(failureThreshold)).To(BeTrue(), "unhealthy.isUnhealthy should have been true")
	g.Expect(healthy.isUnhealthy(failureThreshold)).To(BeFalse(), "healthy.isUnhealthy should have been false")
	t.Log("IsUnhealthy Passed")
}

func TestRestBackoff(t *testing.T) {
	g := NewWithT(t)
	ps := createProbeStatus(0, 0, nil, time.NewTimer(1*time.Minute))
	prevTimer := ps.backOff
	ps.resetBackoff(1 * time.Millisecond)
	g.Expect(prevTimer.Stop()).To(BeFalse(), "RestBackOff should have stopped the existing timer before starting a new one")

	ps = createProbeStatus(0, 0, nil, nil)
	ps.resetBackoff(1 * time.Millisecond)
	g.Expect(ps.backOff).ToNot(BeNil(), "RestBackOff should start a new timer if probestatus backOff is nil")

	ps.backOff.Stop()
	t.Log("ResetBackoff Passed")
}

func TestRecordSuccess(t *testing.T) {
	g := NewWithT(t)
	ps := createProbeStatus(2, 0, nil, time.NewTimer(1*time.Minute))
	successThreshold := 3
	successCount := ps.successCount

	ps.recordSuccess(successThreshold)

	g.Expect(ps.successCount).To(Equal(successCount+1), "RecordSuccess should have incremented success count by 1")
	g.Expect(ps.errorCount).To(BeZero(), "RecordSuccess should have made errorCount equal to 0")
	g.Expect(ps.lastErr).ToNot(HaveOccurred(), "RecordSuccess should have made lastErr equal to nil")

	t.Log("RecordSuccess Passed")
}

func TestRecordFailure(t *testing.T) {
	g := NewWithT(t)
	ps := createProbeStatus(0, 1, nil, nil)
	failureThreshold := 3
	errCount := ps.errorCount
	err := errors.New("failure")

	ps.recordFailure(err, failureThreshold, 0)

	g.Expect(ps.errorCount).To(Equal(errCount+1), "RecordFailure should have incremented the errorCount by 1")
	g.Expect(ps.backOff).To(BeNil(), "RecordFailure should not reset backOff if failureThreshold is not crossed")
	g.Expect(ps.successCount).To(BeZero(), "RecordFailure should have made successCount equal to 0")
	g.Expect(ps.lastErr).To(Equal(err), "RecordFailure should set the lastErr value to the current error")

	ps.recordFailure(err, failureThreshold, 1*time.Minute)

	g.Expect(ps.backOff).ToNot(BeNil(), "RecordFailure should have reset the backOff when failureThreshold is crossed")

	ps.backOff.Stop()
	t.Log("RecordFailure Passed")
}

func TestCanIgnoreProbeError(t *testing.T) {
	g := NewWithT(t)
	ps := createProbeStatus(0, 0, nil, nil)
	err := errors.New("test")

	g.Expect(ps.canIgnoreProbeError(err)).To(BeFalse(), fmt.Sprintf("CanIgnoreProbeError should return false for %v", err))
	g.Expect(ps.canIgnoreProbeError(apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")))).To(BeTrue(),
		"CanIgnoreProbeError should return true for a Forbidden request error")
	g.Expect(ps.canIgnoreProbeError(apierrors.NewUnauthorized("unauthorized"))).To(BeTrue(),
		"CanIgnoreProbeError should return true for an Unauthorized request error")
	g.Expect(ps.canIgnoreProbeError(apierrors.NewTooManyRequests("Too many requests", 10))).To(BeTrue(),
		"CanIgnoreProbeError should return true for a TooManyRequests error")

	t.Log("CanIgnoreProbeError Passed")
}

func TestHandleIgnorableError(t *testing.T) {
	g := NewWithT(t)
	ps := createProbeStatus(0, 0, nil, nil)
	ps.handleIgnorableError(apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")))
	g.Expect(ps.backOff).To(BeNil(),
		"handleIgnorableError should not reset backOff timer for errors other than throttling error.")
	ps.handleIgnorableError(apierrors.NewTooManyRequests("Too many requests", 10))
	g.Expect(ps.backOff).ToNot(BeNil(), "handleIgnorableError should reset backOff timer for throttling error")

	ps.backOff.Stop()
	t.Log("HandleIgnorableError Passed")
}

func createProbeStatus(successCount int, errCount int, lastErr error, backOff *time.Timer) *probeStatus {
	return &probeStatus{successCount: successCount, errorCount: errCount,
		lastErr: lastErr, backOff: backOff}
}
