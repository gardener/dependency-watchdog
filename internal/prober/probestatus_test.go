package prober

import (
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func createProbeStatus(successCount int, errCount int, lastErr error, backOff *time.Timer) *probeStatus {
	return &probeStatus{successCount: successCount, errorCount: errCount,
		lastErr: lastErr, backOff: backOff}
}
func TestIsHealthy(t *testing.T) {
	unhealthy := createProbeStatus(0, 0, nil, nil)
	healthy := createProbeStatus(4, 0, nil, nil)
	successThreshold := 3

	if unhealthy.isHealthy(successThreshold) {
		t.Fatalf("IsHealthy failed, Expected false for unhealthy status, but got true")
	}
	if !healthy.isHealthy(successThreshold) {
		t.Fatalf("IsHealthy failed, Expected true for healthy status, but got false")
	} else {
		t.Log("IsHealthy passed")
	}
}

func TestIsUnhealthy(t *testing.T) {
	unhealthy := createProbeStatus(0, 4, nil, nil)
	healthy := createProbeStatus(0, 2, nil, nil)
	failureThreshold := 3

	if !unhealthy.isUnhealthy(failureThreshold) {
		t.Fatalf("IsUnhealthy failed, Expected true for unhealthy status, but got false")
	}
	if healthy.isUnhealthy(failureThreshold) {
		t.Fatalf("IsUnhealthy failed, Expected false for healthy status, but got true")
	}
	t.Log("IsUnhealthy Passed")
}

func TestRestBackoff(t *testing.T) {
	ps := createProbeStatus(0, 0, nil, time.NewTimer(1*time.Minute))
	prevTimer := ps.backOff
	ps.resetBackoff(1 * time.Millisecond)
	if prevTimer.Stop() {
		t.Fatalf("ResetBackoff failed, Did not stop the existing timer before starting a new one")
	}

	ps = createProbeStatus(0, 0, nil, nil)
	ps.resetBackoff(1 * time.Millisecond)
	if ps.backOff == nil {
		t.Fatalf("ResetBackoff failed, Did not start a new timer for the case where probestatus backoff is nil")
	}

	ps.backOff.Stop()
	t.Log("ResetBackoff Passed")
}

func TestRecordSuccess(t *testing.T) {
	ps := createProbeStatus(2, 0, nil, time.NewTimer(1*time.Minute))
	successThreshold := 3
	successCount := ps.successCount

	ps.recordSuccess(successThreshold)
	if ps.successCount-successCount != 1 {
		t.Fatalf("RecordSuccess failed, Expected success count %v but got %v", successCount+1, ps.successCount)
	}
	if ps.errorCount != 0 {
		t.Fatalf("RecordSuccess failed, Expected error count %v but got %v", 0, ps.errorCount)
	}
	if ps.lastErr != nil {
		t.Fatalf("RecordSuccess failed, Expected lastErr to be nil but go %v", ps.lastErr)
	}
	t.Log("RecordSuccess Passed")
}

func TestRecordFailure(t *testing.T) {
	ps := createProbeStatus(0, 1, nil, nil)
	failureThreshold := 3
	errCount := ps.errorCount
	err := errors.New("failure")

	ps.recordFailure(err, failureThreshold, 0)
	if ps.errorCount != errCount+1 {
		t.Fatalf("RecordFailure failed, Expected errorCount %v, but got %v", errCount+1, ps.errorCount)
	}
	if ps.backOff != nil {
		t.Fatalf("RecordFailure failed, Expected backOff to be nil but go %v", ps.backOff)
	}
	if ps.successCount != 0 {
		t.Fatalf("RecordFailure failed, Expected successCount to be %v but got %v", 0, ps.successCount)
	}
	if ps.lastErr != err {
		t.Fatalf("RecordFailure failed, Expected last error to be %v but got %v", err, ps.lastErr)
	}

	ps.recordFailure(err, failureThreshold, 1*time.Minute)
	if ps.backOff == nil {
		t.Fatalf("RecordFailure failed, Got nil backOff, expected a non nil value")
	}

	ps.backOff.Stop()
	t.Log("RecordFailure Passed")
}

func TestCanIgnoreProbeError(t *testing.T) {
	ps := createProbeStatus(0, 0, nil, nil)
	err := errors.New("test")
	if ps.canIgnoreProbeError(err) {
		t.Fatalf("CanIgnoreProbeError failed, Expected false for error %v but got true", err)
	}
	if !ps.canIgnoreProbeError(apierrors.NewNotFound(schema.GroupResource{}, "test")) {
		t.Fatalf("CanIgnoreProbeError failed, Expected true for a notFound error but got false")
	}
	if !ps.canIgnoreProbeError(apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden"))) {
		t.Fatalf("CanIgnoreProbeError failed, Expected true for a forbidden request error but got false")
	}
	if !ps.canIgnoreProbeError(apierrors.NewUnauthorized("unauthorized")) {
		t.Fatalf("CanIgnoreProbeError failed, Expected true for an unauthorized request error but got false")
	}
	if !ps.canIgnoreProbeError(apierrors.NewTooManyRequests("Too many requests", 10)) {
		t.Fatalf("CanIgnoreProbeError failed, Expected true for too many requests error type but got false")
	}
	if ps.backOff == nil {
		t.Fatalf("CanIgnoreProbeError failed, Expected backOff to be non-nil but got nil")
	}

	ps.backOff.Stop()
	t.Log("CanIgnoreProbeError Passed")
}
