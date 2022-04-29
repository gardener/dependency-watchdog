package prober

import "time"

const (
	defaultDependentResourceCheckTimeout  = 20 * time.Millisecond
	defaultDependentResourceCheckInterval = 5 * time.Millisecond
)

type scalerOption func(options *scalerOptions)

type scalerOptions struct {
	dependentResourceCheckTimeout  *time.Duration
	dependentResourceCheckInterval *time.Duration
}

func buildScalerOptions(options ...scalerOption) *scalerOptions {
	opts := new(scalerOptions)
	for _, opt := range options {
		opt(opts)
	}
	fillDefaultsOptions(opts)
	return opts
}

func withDependentResourceCheckTimeout(timeout time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.dependentResourceCheckTimeout = &timeout
	}
}

func withDependentResourceCheckInterval(interval time.Duration) scalerOption {
	return func(options *scalerOptions) {
		options.dependentResourceCheckInterval = &interval
	}
}

func fillDefaultsOptions(options *scalerOptions) {
	if options.dependentResourceCheckTimeout == nil {
		options.dependentResourceCheckTimeout = new(time.Duration)
		*options.dependentResourceCheckTimeout = defaultDependentResourceCheckTimeout
	}
	if options.dependentResourceCheckInterval == nil {
		options.dependentResourceCheckInterval = new(time.Duration)
		*options.dependentResourceCheckInterval = defaultDependentResourceCheckInterval
	}
}
