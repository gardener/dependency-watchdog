# Probe Status & Transitions

## Overview

This document describes the transitions of status of a probe and actions taken when a probe succeeds or fails.

## Probe status
For each probe a success and a failure threshold is defined.
An Operator can choose to define custom values by setting them as part of the [`ConfigMap`](/example/04-dwd-prober-configmap.yaml) which is used by `DWD`. The configuration will be mapped to the following `struct`.

```go
type Config struct {
	Name                                string                  `yaml:"name"`
	Namespace                           string                  `yaml:"namespace,omitempty"`
	InternalKubeConfigSecretName        string                  `yaml:"internalKubeConfigSecretName"`
	ExternalKubeConfigSecretName        string                  `yaml:"externalKubeConfigSecretName"`
	ProbeInterval                       *time.Duration          `yaml:"probeInterval,omitempty"`
	InitialDelay                        *time.Duration          `yaml:"initialDelay,omitempty"`
	SuccessThreshold                    *int                    `yaml:"successThreshold,omitempty"`
	FailureThreshold                    *int                    `yaml:"failureThreshold,omitempty"`
	InternalProbeFailureBackoffDuration *time.Duration          `yaml:"internalProbeFailureBackoffDuration,omitempty"`
	BackoffJitterFactor                 *float64                `yaml:"backoffJitterFactor,omitempty"`
	DependentResourceInfos              []DependentResourceInfo `yaml:"dependentResourceInfos"`
}
```
The default values of thresholds are:
1. `SuccessThreshold`:  1
2. `FailureThreshold`: 3


## Probe status transitions

Each probe maintains its status via the following struct:

```go
type probeStatus struct {
	successCount int
	errorCount   int
	lastErr      error
	backOff      *time.Timer
}
```
If a probe is successful then its `successCount` will be incremented. It can have an max value as defined in `SuccessThreshold`. Similarly if the probe is unsuccessful its `errorCount` will be incremented. It can have a max value as defined in `FailureThreshold`.

Only if a probe has a `successCount` >= `SuccessThreshold` will it be considered as `Healthy`. Similarly a probe will be considered `Unhealthy or Failed` if its `errorCount` >= `FailureThreshold`.

Following table shows how the probe's `successCount`, `errorCount` and its computed state changes:

> Let's assume that `SuccessThreshold=1` and `FailureThreshold=3`. The below table only provides a subset of examples to demonstrate the way the success/error counts change and how it reflects in its health status. The below list is not comprehensive. 

| previous probe status counts | current probe result | current probe status counts | isHealthy | isUnhealthy |
| :----------- | :--------- | :----- | :----- | :----- | 
| {successCount: 0, errorCount: 0} | success | {successCount: 1, errorCount: 0} | true | false |
| {successCount: 1, errorCount: 0} | failure | {successCount: 0, errorCount: 1} | false | false |
| {successCount: 0, errorCount: 2} | failure | {successCount: 0, errorCount: 3} | false | true |
| {successCount: 0, errorCount: 3} | success | {successCount: 1, errorCount: 0} | true | false |


### Probe failure identification

An error returned from a probe run can be categorized into two types:

**Transient Errors**
These errors are transient in nature and its assumed that in subsequent runs of the probe these errors will not be seen. Currently the following errors fall under this category:
1. Requests to the Kube-API-Server are throttled and results in `TooManyRequests` error.
2. Due to secret rotation its possible that the call to the Kube-API-Server fails with either `Forbidden` or `Unauthorized` error.

If any of the above errors are encountered during a probe run, then the `errorCount` is not incremented. Last known state is retained as the state of the Kube-API-Server could not be determined.

**Non-Transient Errors**
Any error which is not `Transient Error` falls under this category. If such an error is encountered during a probe run then `errorCount` will be incremented.