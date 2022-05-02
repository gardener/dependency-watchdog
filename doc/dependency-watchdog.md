# Dependency Watchdog Redesign

## Table of Contents

- [Dependency Watchdog Redesign](#dependency-watchdog-redesign)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Out of scope](#out-of-scope)
  - [Proposal](#proposal)
    - [Prober Configuration](#prober-configuration)
      - [Scaling level](#scaling-level)
    - [Prober lifecycle](#prober-lifecycle)
      - [Creation of a probe](#creation-of-a-probe)
      - [Removal of a probe](#removal-of-a-probe)
    - [Scaler Flow](#scaler-flow)
    - [Metrics](#metrics)

## Summary

The current design of Dependency-Watchdog (a.k.a `DWD`) has flaws which results in race conditions, non-deterministic
behavior due to heavy usage of `un-substantiated` sleep and timeout values and complex goroutine handling leading to
inconsistent behavior w.r.t scaling of dependent resource. The proposal is to revamp the design and make it testable and
deterministic.

## Motivation

DWD originally only used to handle the scaling up/down of `KCM` in case the shoot API service is unreachable via
external probe (the same API server endpoint that is used by shoot's kubelet). In the recent past other control plane
components like `Machine Controller Manager` and `Cluster Autoscaler` have been added.

The current design introduces unnecessary complexity w.r.t:

* Management of probes @see [#36](https://github.com/gardener/dependency-watchdog/issues/36)
* Handling of changes to kubeconfig secrets in the shoot namespaces
* Managing dependency graph deterministically @see [#38](https://github.com/gardener/dependency-watchdog/issues/38)
* Properly handle cluster state (especially for clusters marked for hibernation or waking from hibernation
  @see [#45](https://github.com/gardener/dependency-watchdog/issues/45)
* There is an epic [gardener#4251](https://github.com/gardener/gardener/issues/4251) which aims to transition
  controllers to use the `controller-runtime` components. As part of this initiative we will also move the DWD to use
  the `controller-runtime` components.

Last but not the least, today the code complexity of DWD means that we cannot have sufficient unit and integration tests
leading to a lot of manual effort in testing and leaving DWD codebase vulnerable.

### Goals

* Simplify the management of probes to ensure deterministic and non-competing behavior w.r.t scaling of dependent
  control plane components
* Simplify and enhance the dependency management allowing concurrent scale Up/Down of dependent resources
* Use the controller-runtime components
* Achieve more than 80% code coverage via unit tests and introduce integration tests
* Define and expose metrics and create dashboards for the exposed metrics

### Out of scope

DWD currently only scales dependent control plane components based on the result of internal and external probe to the
shoot API server. It is not the intent of this proposal to allow consumers to define custom probe endpoint(s) other than
API server probes. If this generality is required then further changes can be taken up at a later point in time.

## Proposal

Several changes are proposed in the design of the prober.

### Prober Configuration

To allow both sequential (ordered) and concurrent scaling up/down of resources we propose to change to the prober
configuration. Following will be the new configuration:

```yaml
# values are only representational and do not represent default values
name: "shoot-kube-apiserver"
namespace: ""
# secret names for internal and external probes
internalKubeConfigSecretName: "dws-interal-probe-secret-name"
externalKubeConfigSecretName: "dwd-external-probe-secret-name"
# prober configuration, defaults will be assumed for all optional configuration
probeInterval: 20s # optional, represents the interval with which the prober will probe a shoot API server 
initialDelay: 5s # optional, represents an initial delay to start the first probe
successThreshold: 1 # optional, how many consecutive successful attempts does it take to declare a probe healthy
failureThreshold: 3 # optional, how many consecutive failed attempts does it take to declare a probe as failed
internalProbeFailureBackOffDuration: 30s # optional, in case there is a failure to probe the API server via internal probe, an optional backoff can be configured
backOffJitterFactor: 0.2 # optional, jitter introduced in probeInterval

# ------------------------------------------------------------
# dependent resource infos contain information about resources that needs to be scaled up or down. Provision has been made to allow one or more resources to be scaled down/up concurrently by introducing levels.
# each dependent resource that must have both scaleUp and scaleDown configuration specified
dependentResourceInfos:
  - ref: # provides a reference (identifier) to a resource that is a target of scaling
      kind: "Deployment"
      name: "kube-controller-manager"
      apiVersion: "apps/v1"
    scaleUp: # provides scale-up configuration
      level: 1 # explained below
      initialDelay: 10s # optional, initial delay before the scaleUp begins
      timeout: 60s # optional, total timeout to wait for the scale operation to update the scale sub-resource
      replicas: 1 # number of replicas to scale-up to
    scaleDown: # provides scale-down configuration
      level: 0 # explained below
      initialDelay: 15s # optional, initial delay before the scaleDown begins
      timeout: 45s # optional, total timeout to wait for the scale operation to update the scale sub-resource
      replicas: 0 # number of replicas to scale-down to
  - ref:
      kind: "Deployment"
      name: "machine-controller-manager"
      apiVersion: "apps/v1"
    scaleUp:
      level: 1
      initialDelay: 10s
      timeout: 60s
      replicas: 1
    scaleDown:
      level: 0
      initialDelay: 15s
      timeout: 45s
      replicas: 0
  - ref:
      kind: "Deployment"
      name: "cluster-autoscaler"
      apiVersion: "apps/v1"
    scaleUp:
      level: 0
      initialDelay: 10s
      timeout: 60s
      replicas: 1
    scaleDown:
      level: 1
      initialDelay: 15s
      timeout: 45s
      replicas: 0

```

#### Scaling level

Each dependent resource that should be scaled up or down is associated to a level. Levels are ordered and processed in
ascending order (starting with 0). In the above sample configuration for a `Scale-down` operation
both `kube-controller-manager` and `machine-controller-manager`  will be scale down first concurrently (both are at the
same level). Only once they have been successfully scaled down will `cluster-autoscaler` which is level 1 be scaled
down.

### Prober lifecycle

In the current code probers for each shoot cluster were destroyed and re-created upon receipt of CRUD events for
internal/external secrets for the shoot and also for cluster update events. This was done asynchronously and resulted in
many edge cases where existing probes were not cleanly shutdown and in some cases multiple mutually-cancelling probes (
old and new) for the same shoot were also seen causing underministic behavior which could not be self corrected.

In the new proposal we have attempted to significantly simplify the lifecycle of a probe for a shoot cluster.
A reconciler which is registered to a controller-runtime `Manager` will be only listening actively for CRUD events
on `Cluster` resources.

#### Creation of a probe

When a `Reconciler` receives a request for a `Cluster` change, it will query the extension kube-api server to get
the `Cluster` resource.
If there is no existing running probe found, it will create a new probe, only if the cluster state is not `hibernated`
OR if `cluster.shoot.spec.hibernation.enabled` is not set to true.

#### Removal of a probe

There are only two events which can cause an active probe to be removed for a shoot cluster.

1. `DeletionTimestamp` is being set on the cluster indicating that this cluster has been marked for deletion.
2. Cluster has been marked for `hibernation`.

If the `Reconciler` queries the extension kube-api server and if the `Cluster` object returned satisfies any of the
above conditions then
any existing probe for this shoot cluster will be cancelled.

> **NOTE:**
>
>It is possible that prober scales down KCM/MCM/CA partially or fully before the reconciler reacts to a `Request` and
> then attempts to cancel the probe because the cluster has been marked
> for hibernation. This will cause the hibernation to be stuck as it requires KCM/MCM to be up and running.
> See [Issue#45](https://github.com/gardener/dependency-watchdog/issues/45) for the conclusion where it
> was decided that `Prober` should not be responsible to scale `MCM/KCM` back up to ensure that the hibernation
> completes.
>
> **Reasoning:**
>
> If `prober` takes the responsibility to ensure that `KCM/MCM` are scaled back up for clusters for which `hibernation`
> has been enabled, then there is
> one edge case case where `cluster.shoot.spec.hibernation.enabled` is set to true but `cluster.shoot.status.hibernated`
> is false indicating that the hibernation is enabled
> but not yet completed. In this scenario there are 2 actors, namely `gardenlet` and `DWD prober` which now compete with
> each other. `Prober` sees that hibernation has been enabled
> but not yet complete and `KCM/MCM` are down, so it tries to bring them back up. However, it is possible that all shoot
> machines have been successfully stopped and `gardenlet` is in the process
> of stopping shoot control plane components. This causes possibility of an inconsistent state where two actors are
> trying to execute conflicting operations.
> Technical we can handle this as part of DWD but then it would mean that it will need to know if all shoot machines
> have been successfully stopped adding direct dependency to `MCM` APIs.
>
> Therefore, the decision was to only cancel the probe if the cluster has been marked for `hibernation`. 
> An [gardener#5878](https://github.com/gardener/gardener/issues/5878) has been raised to enhance reconciliation done in gardener.
>

### Scaler Flow

Resources to be scaled and the order in which the scaling needs to be done in defined as part of a `ConfigMap` which is created in the `garden` namespace at the time of seed creation. This configuration does not change during the lifetime of DWD `Prober`. The configuation now allows concurrent scale up or scale down of resources. 

DWD now represents the dependent resources, their start order and their relative dependencies by representing it as a `Graph`. It leverages an existing [flow](https://github.com/gardener/gardener/tree/master/pkg/utils/flow) framework to do the following:
1. Create the flow consisting of scale up/down of dependent resources at the time of creation of a probe for a shoot cluster.
2. Utilizes `taskFn.Parallel` to model actions on dependent resources having the same `Level`.
3. Utilizes the capability to specify `Dependencies` between two nodes in a `Graph` to establish clear order for scaling operations.

It is now possible to unit test creation of the flow to ensure that proper order has been set and concurrent tasks are properly set as well. Additional unit tests have been created which programmatically brings up a vanilla `KIND` cluster to test the `Flow`s.

### Metrics

At present DWD does not expose any metrics. We propose to capture and expose the following metrics:

**Metrics across all shoot clusters in the `Seed` cluster**

* Total number of active probes that are running
* Total number of requests made to the Kube API server across all probers
* Total number of scaling operations initiated by any prober. These can either be a scale-up or scale-down operation
* Total number of times Kube API server has returned `To Many Requests` error, in other words the request to the Kube
  API server has been throttled.

**Metrics per shoot**

* Total number of internal probe failures
* Total number of external probe failures
* Total number of scaling (Up/Down) attempts

TODO: Find out where the changes needs to be made in `gardener/gardener`.

