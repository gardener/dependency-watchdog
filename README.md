# Dependency Watchdog

<img src="logo/gardener-dwd.png" style="width:200px">

[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/dependency-watchdog)](https://goreportcard.com/report/github.com/gardener/dependency-watchdog)
[![GoDoc](https://godoc.org/github.com/gardener/dependency-watchdog?status.svg)](https://pkg.go.dev/github.com/gardener/dependency-watchdog)


## Overview
A watchdog which actively looks out for disruption and recovery of critical services. If there is a disruption then it will prevent cascading failure by conservatively scaling down dependent configured services and if a critical service has just recovered then it will expedite the recovery of dependent services/pods.

Avoiding cascading failure is handled by `Prober` component and expediting recovery of dependent services/pods is handled by `Weeder` component. These are separately deployed as individual pods.

## Dependency Watchdog Prober

Prober periodically probes Kube ApiServer via two separate probes:
1.  Internal Probe: Local cluster DNS name which resolves to the ClusterIP of the Kube Apiserver
2.  External Probe: DNS name via which the kubelet running in each node in the data plane (a.k.a shoot in gardener terminology) communicates to the Kube Apiserver running in its control plane (a.k.a seed in gardener terminology)

If the internal probe fails then it skips the external probe as it indicates that the Kube ApiServer is down. In case the internal probe succeeds then it probes using the external probe. If the external probe fails consecutively for more than `failureThreshold` times then it assumes that the kubelet running in the nodes of the shoot cluster will be unable to renew their leases. This will have a cascading effect as Kube Controller Manager will transtion the status of the nodes to `Unknown`. This status change will be observed by the Machine Controller Manager which manages the lifecycle of the machines for the shoot cluster. After waiting rfor a configure period, Machine controller manager will trigger drain for these nodes and will subsequently stop these machines once new machines are launched.

To prevent downtime to consumer workloads that are running on the nodes of the shoot cluster in case the Kube Apiserver is not reachable prober will initiate a scale down of services which will react to the nodes not been able to renew their lease. In case of Gardener these comprise of Kube Controller Manager, Machine Controller Manager and Cluster Autoscaler. Consumers can configure the deployments that needs to be scaled down.

### Prober Configuration

Consumers can configure the prober by creating a kubernetes ConfigMap and set the serialized YAML configuration as `data`. Example configuration can be referenced [here]().

| Property | Type | Required?| Default Value | Description |
|  :-----  | :--- | :----- | :---- | :---- |
| internalKubeConfigSecretName | string | Yes | NA |  name of the kubernetes secret which will contain the internal kubeconfig to connect to the Kube Apiserver of the shoot control plane |
| externalKubeConfigSecretName | string | Yes | NA | name of the kubernetes secret which will contain the external kubeconfig to connect to the Kube Apiserver of the shoot control plane |
| probeInterval | metav1.Duration | No | 10s | frequency with which prober will probe the shoot control plane Kube Apiserver |
| initialDelay | metav1.Duration | No | 30s | initial delay after which the probe starts |
| successThreshold | int | No | 1 | consecutive number of successful probes required to transition the probe status to success |

### Prober Golang API

< TODO >

**Gardener Setup**

Dependency watchdog prober runs as a central component in the garden namespace of a seed cluster. A seed cluster hosts the control planes for several shoot clusters setup and running in dedicated shoot-control namespaces. Prober will have access to the internal and external KubeConfig required to connect to the Kube Apiserver runnning in each of the shoot-control namespaces. A dedicated prober instance will be created per shoot which will run asynchronously as long as the shoot control plane is up and running.


## Dependency Watchdog Weeder



## How to contribute?

Contributions are always welcome!

In order to contribute ensure that you have the development environment setup and you familiarize yourself with required steps to build, verify-quality and test.

### Setting up development environment

**Installing Golang**

Minimum Golang version required: `1.18`.

If you do not have golang setup then follow the [installation instructions](https://go.dev/doc/install).

**Installing Git**

Git is used as version control for dependency-watchdog. If you do not have git installed already then please follow the [installation instructions](https://git-scm.com/downloads).

### Raising a Pull Request

To raise a pull request do the following:
1. Create a fork of [dependency-watchdog](https://github.com/gardener/dependency-watchdog)
2. Add [dependency-watchdog](https://github.com/gardener/dependency-watchdog) as upstream remote via 
 ```bash 
    git remote add upstream https://github.com/gardener/dependency-watchdog
 ```
3. It is recommended that you create a git branch and push all your changes for the pull-request.
4. Ensure that while you work on your pull-request, you continue to rebase the changes from upstream to your branch. To do that execute the following command:
```bash
   git pull --rebase upstream master
```
5. We prefer clean commits. If you have multiple commits in the pull-request, then squash the commits to a single commit. You can do this via `interactive git rebase` command. For example if your PR branch is ahead of remote origin HEAD by 5 commits then you can execute the following command and pick the first commit and squash the remaining commits.
```bash
   git rebase -i HEAD~5 #actual number from the head will depend upon how many commits your branch is ahead of remote origin master
```

### Testing Dependency-Watchdog on Local Gardener

**Local Gardener Setup**

Dependency watchdog pods are already baked into the local gardener setup using KIND and local docker daemon. To install local gardener follow the [setup guide](https://github.com/gardener/gardener/blob/master/docs/deployment/getting_started_locally.md). This will provision two pods `dependency-watchdog-probe-*` and `dependency-watchdog-weed-*` in the `garden` namespace of the local seed cluster.

**Testing local changes to Dependency Watchdog components**

< TODO >

## Feedback and Support

We always look forward to active community engagement.

Please report bugs or suggestions on how we can enhance `dependency-watchdog` to address additional recovery scenarios on [GitHub issues](https://github.com/gardener/dependency-watchdog/issues)