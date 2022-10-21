# Dependency Watchdog

<img src="logo/gardener-dwd.png" style="width:200px">

[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/)
[![Unit Tests](https://testgrid.k8s.io/q/summary/gardener-dependency-watchdog/ci-dependency-watchdog-unit/tests_status?style=svg)](https://testgrid.k8s.io/q/summary/gardener-dependency-watchdog/ci-dependency-watchdog-unit/tests_status)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/dependency-watchdog)](https://goreportcard.com/report/github.com/gardener/dependency-watchdog)
[![GoDoc](https://godoc.org/github.com/gardener/dependency-watchdog?status.svg)](https://pkg.go.dev/github.com/gardener/dependency-watchdog)

## Overview
A watchdog which actively looks out for disruption and recovery of critical services. If there is a disruption then it will prevent cascading failure by conservatively scaling down dependent configured services and if a critical service has just recovered then it will expedite the recovery of dependent services/pods.

Avoiding cascading failure is handled by `Prober`(/docs/concepts/prober.md)  component and expediting recovery of dependent services/pods is handled by `Weeder`(/docs/concepts/weeder.md)  component. These are separately deployed as individual pods.

### Current Limitation & Future Scope
Although in the current offering the `Prober` is tailored to handle one such use case of `kube-apiserver` connectivity, but the usage of prober can be extended to solve similar needs for other scenarios where the components involved might be different.

## Start using or developing the Dependency Watchdog

See our documentation in the /docs repository, please [find the index here](docs/README.md).

## Feedback and Support

We always look forward to active community engagement.

Please report bugs or suggestions on how we can enhance `dependency-watchdog` to address additional recovery scenarios on [GitHub issues](https://github.com/gardener/dependency-watchdog/issues)