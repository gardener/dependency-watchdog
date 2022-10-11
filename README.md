# Dependency Watchdog

<img src="logo/gardener-dwd.png" style="width:200px">

[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/dependency-watchdog-master/jobs/master-head-update-job/)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/dependency-watchdog)](https://goreportcard.com/report/github.com/gardener/dependency-watchdog)
[![GoDoc](https://godoc.org/github.com/gardener/dependency-watchdog?status.svg)](https://pkg.go.dev/github.com/gardener/dependency-watchdog)


## Overview
A watchdog which actively looks out for disruption and recovery of critical services. If there is a disruption then it will prevent cascading failure by conservatively scaling down dependent configured services and if a critical service has just recovered then it will expedite the recovery of dependent services/pods.

Avoiding cascading failure is handled by `Prober` component and expediting recovery of dependent services/pods is handled by `Weeder` component. These are separately deployed as individual pods.

## Dependency Watchdog Prober




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