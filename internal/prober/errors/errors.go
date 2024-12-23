// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import "fmt"

// ErrorCode is the type for error codes.
type ErrorCode string

const (
	// ErrProbeAPIServer is the error code for errors in the API server probe.
	ErrProbeAPIServer = "ERR_PROBE_API_SERVER"
	// ErrSetupProbeClient is the error code for errors in setting up the probe client.
	ErrSetupProbeClient = "ERR_SETUP_PROBE_CLIENT"
	// ErrProbeNodeLease is the error code for errors in the node lease probe.
	ErrProbeNodeLease = "ERR_PROBE_NODE_LEASE"
	// ErrScaleUp is the error code for errors in scaling up the dependent resources
	ErrScaleUp = "ERR_SCALE_UP"
	// ErrScaleDown is the error code for errors in scaling down the dependent resources
	ErrScaleDown = "ERR_SCALE_DOWN"
)

// ProbeError is the error type for probe errors. It contains the error code, the cause of the error, and the error message.
// It is used by prober to record its last error and is currently only used for unit tests.
type ProbeError struct {
	// Code is the error code that is returned by the probe.
	Code ErrorCode
	// Cause is the error that happened during the probe.
	Cause error
	// Message is used for mentioning additional details describing the error.
	Message string
}

// Error is the error interface implementation for ProbeError.
func (e *ProbeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("Code: %s, Message: %s, Cause: %s", e.Code, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("Code: %s, Message: %s", e.Code, e.Message)
}

// WrapError wraps an error with an error code and a message.
func WrapError(err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}
	return &ProbeError{
		Code:    code,
		Cause:   err,
		Message: message,
	}
}
