# Dependency Watchdog

<img src="logo/gardener-dwd.png" style="width:200px">

[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/dependency-watchdog)](https://goreportcard.com/report/github.com/gardener/dependency-watchdog)
[![GoDoc](https://godoc.org/github.com/gardener/dependency-watchdog?status.svg)](https://pkg.go.dev/github.com/gardener/dependency-watchdog)


## Overview
A watchdog which actively looks out for disruption and recovery of critical services. If there is a disruption then it will prevent cascading failure by conservatively scaling down dependent configured services and if a critical service has just recovered then it will expedite the recovery of dependent services/pods.

Avoiding cascading failure is handled by `Prober` component and expediting recovery of dependent services/pods is handled by `Weeder` component. These are separately deployed as individual pods.

## Start using or developing the Dependency Watchdog

See our documentation in the /docs repository, please [find the index here](docs/README.md).


**Gardener Setup**

Dependency watchdog prober runs as a central component in the garden namespace of a seed cluster. A seed cluster hosts the control planes for several shoot clusters setup and running in dedicated shoot-control namespaces. Prober will have access to the internal and external KubeConfig required to connect to the Kube Apiserver runnning in each of the shoot-control namespaces. A dedicated prober instance will be created per shoot which will run asynchronously as long as the shoot control plane is up and running.


## Feedback and Support

We always look forward to active community engagement.

Please report bugs or suggestions on how we can enhance `dependency-watchdog` to address additional recovery scenarios on [GitHub issues](https://github.com/gardener/dependency-watchdog/issues)