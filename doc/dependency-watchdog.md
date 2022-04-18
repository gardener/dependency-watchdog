# Dependency Watchdog Redesign

## Table of Contents

- [Dependency Watchdog Redesign](#dependency-watchdog-redesign)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
  - [Alternatives](#alternatives)

## Summary

The current design of Dependency-Watchdog (a.k.a `DWD`) has flaws which results in race conditions, non-deterministic behavior due to heavy usage of `un-substantiated` sleep and timeout values and complex goroutine handling leading to inconsistent behavior w.r.t scaling of dependent resource. The proposal is to revamp the design and make it testable and deterministic.

## Motivation
DWD originally only used to handle the scaling up/down of `KCM` in case the shoot API service is unreachable via external probe (the same API server endpoint that is used by shoot's kubelet). In the recent past other control plane components like `Machine Controller Manager` and `Cluster Autoscaler` have been added.

The current design introduces unnecessary complexity w.r.t:
* Management of probes @see [#36](https://github.com/gardener/dependency-watchdog/issues/36)
* Handling of changes to kubeconfig secrets in the shoot namespaces
* Managing dependency graph deterministically @see [#38](https://github.com/gardener/dependency-watchdog/issues/38)
* Properly handle cluster state (especially for clusters marked for hibernation or waking from hibernation @see [#45](https://github.com/gardener/dependency-watchdog/issues/45)
* There is an epic [gardener#4251](https://github.com/gardener/gardener/issues/4251) which aims to transition controllers to use the `controller-runtime` components. As part of this initiative we will also move the DWD to use the `controller-runtime` components.

Since DWD now also handles scaling extensions (e.g MCM), it has to set the dependent configuration for MCM in the `ConfigMap` which is done as part of gardener core. Gardener neither has the knowledge nor should have any knowledge of extensions. @see [#42](https://github.com/gardener/dependency-watchdog/issues/42). This requires a design change in DWD and potentially in other extension specific controller.

Last but not the least, today the code complexity of DWD means that we cannot have sufficient unit and integration tests leading to a lot of manual effort in testing and leaving DWD codebase vulnerable.

### Goals

* Simplify the management of probes to ensure deterministic and non-competing behavior w.r.t scaling of dependent control plane components
* Simplify and enhance the dependency management allowing concurrent scale Up/Down of dependent resources
* De-link gardener core from extension specific configurations and allow shoot specific setup especially for usage externally where MCM might be used to manage machine deployments.
* Use the controller-runtime components

### Non-Goals

DWD currently only scales dependent control plane components based on the result of external probe to the shoot API server. It is not the intent of this proposal to allow consumers to define custom probe endpoint(s), thereby making it very generic.

## Proposal

## Alternatives