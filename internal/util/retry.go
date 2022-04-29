package util

import (
	"context"
	"time"
)

type RetryResult[T any] struct {
	Value T
	Err   error
}

func Retry[T any](ctx context.Context, operation string, fn func() (T, error), numAttempts int, backOff time.Duration, canRetry func(error) bool) RetryResult[T] {
	var result T
	var err error
	for i := 1; i <= numAttempts; i++ {
		select {
		case <-ctx.Done():
			logger.Error(ctx.Err(), "context has been cancelled, stopping retry", "operation", operation)
			return RetryResult[T]{Err: ctx.Err()}
		default:
		}
		result, err = fn()
		if err == nil {
			return RetryResult[T]{Value: result, Err: err}
		}
		if !canRetry(err) {
			logger.Error(err, "exiting retry as canRetry has returned false", "operation", operation, "exitOnAttempt", i)
			return RetryResult[T]{Err: err}
		}
		select {
		case <-ctx.Done():
			logger.Error(ctx.Err(), "context has been cancelled, stopping retry", "operation", operation)
			return RetryResult[T]{Err: ctx.Err()}
		case <-time.After(backOff):
			logger.V(4).Info("will attempt to retry operation", "operation", operation, "currentAttempt", i, "error", err)
		}
	}
	return RetryResult[T]{Value: result, Err: err}
}

func RetryUntilPredicate(ctx context.Context, operation string, predicateFn func() bool, timeout time.Duration, interval time.Duration) bool {
	timer := time.NewTimer(timeout)
	for {
		select {
		case <-ctx.Done():
			logger.V(4).Info("context has been cancelled, exiting retrying operation", "operation", operation)
			return false
		case <-timer.C:
			logger.V(4).Info("timed out waiting for predicateFn to be true", "operation", operation)
			return false
		default:
			if predicateFn() {
				return true
			}
			time.Sleep(interval)
		}
	}
}

func AlwaysRetry(err error) bool {
	return true
}
