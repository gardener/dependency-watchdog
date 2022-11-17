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

package prober

import (
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type probeStatus struct {
	successCount int
	errorCount   int
	lastErr      error
	backOff      *time.Timer
}

func (ps *probeStatus) canIgnoreProbeError(err error) bool {
	// we now create new shoot client by fetching the secret for every probe, we can ignore an error where probes fail due to authentication/authorization failures
	secretsRotated := apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
	apiServerThrottledRequests := apierrors.IsTooManyRequests(err)
	return secretsRotated || apiServerThrottledRequests
}

func (ps *probeStatus) handleIgnorableError(err error) {
	// if kube API server throttled requests then we should back-off a bit
	apiServerThrottledRequests := apierrors.IsTooManyRequests(err)
	if apiServerThrottledRequests {
		ps.resetBackoff(backOffDurationForThrottledRequests)
	}
}

func (ps *probeStatus) recordFailure(err error, failureThreshold int, failureThresholdBackoffDuration time.Duration) {
	if ps.errorCount < failureThreshold {
		ps.errorCount++
	}
	ps.lastErr = err
	ps.successCount = 0
	if ps.isUnhealthy(failureThreshold) {
		ps.resetBackoff(failureThresholdBackoffDuration)
	}
}

func (ps *probeStatus) recordSuccess(successThreshold int) {
	ps.errorCount = 0
	ps.lastErr = nil
	if ps.successCount < successThreshold {
		ps.successCount++
	}
	ps.resetBackoff(0)
}

func (ps *probeStatus) resetBackoff(d time.Duration) {
	if ps.backOff != nil {
		ps.backOff.Stop()
	}
	ps.backOff = time.NewTimer(d)
}

func (ps *probeStatus) isHealthy(successThreshold int) bool {
	return ps.successCount >= successThreshold
}

func (ps *probeStatus) isUnhealthy(failureThreshold int) bool {
	return ps.errorCount >= failureThreshold
}
