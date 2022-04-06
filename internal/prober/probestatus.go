package prober

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"time"
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
	// if kube API server throttled requests then we should back-off a bit
	apiServerThrottledRequests := apierrors.IsTooManyRequests(err)
	if apiServerThrottledRequests {
		ps.resetBackoff(backOffDurationForThrottledRequests)
	}
	return apiServerThrottledRequests || apierrors.IsNotFound(err) || secretsRotated
}

func (ps *probeStatus) recordFailure(err error, failureThreshold int) {
	if ps.errorCount <= failureThreshold {
		ps.errorCount++
	}
	ps.lastErr = err
	ps.successCount = 0
	if ps.isUnhealthy(failureThreshold) {
		ps.resetBackoff(internalProbeUnhealthyBackoffDuration)
	}
}

func (ps *probeStatus) recordSuccess(successThreshold int) {
	ps.errorCount = 0
	ps.lastErr = nil
	if ps.successCount <= successThreshold {
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
