package errors

import "fmt"

type ErrorCode string

const (
	ErrProbeAPIServer   = "ERR_PROBE_API_SERVER"
	ErrSetupProbeClient = "ERR_SETUP_PROBE_CLIENT"
	ErrProbeNodeLease   = "ERR_PROBE_NODE_LEASE"
	ErrScaleUp          = "ERR_SCALE_UP"
	ErrScaleDown        = "ERR_SCALE_DOWN"
)

type ProbeError struct {
	Code    ErrorCode
	Cause   error
	Message string
}

func (e *ProbeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("Code: %s, Message: %s, Cause: %s", e.Code, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("Code: %s, Message: %s", e.Code, e.Message)
}

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
