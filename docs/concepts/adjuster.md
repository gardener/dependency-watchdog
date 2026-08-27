# Adjuster

## Overview

The current purpose of the Adjuster is to ensure that `machine-controller-manager` operator that is present across shoot
clusters is directed to allow sufficient time for [Machines](https://pkg.go.dev/github.com/gardener/machine-controller-manager@v0.62.1/pkg/apis/machine/v1alpha1#Machine) 
of a [MachineDeployment](https://pkg.go.dev/github.com/gardener/machine-controller-manager@v0.62.1/pkg/apis/machine/v1alpha1#MachineDeployment) to becoming `Running`
and join the shoot cluster.

Adjuster observes specific failures of
`machine-controller-manager` [Machine](https://pkg.go.dev/github.com/gardener/machine-controller-manager@v0.62.1/pkg/apis/machine/v1alpha1#Machine)
objects for every shoot cluster of the seed. If number of Machines that `Failed` to join the cluster cross the `MachineFailureThresholdFraction` (configurable), 
then the adjuster increases the [effective-creation-timeout](https://github.com/gardener/machine-controller-manager/issues/1098)
of the parent `MachineDeployment` for all `Machines` belonging to the same `instanceType` and `zone` 
(called `ProvisionKey`) across all shoots of the seed.

![Adjuster High Level](./content/adjuster-high-level.svg)

NOTE: The current implementation directly watches [Machine](https://pkg.go.dev/github.com/gardener/machine-controller-manager@v0.62.1/pkg/apis/machine/v1alpha1#Machine) of shoot clusters. In the future, this will be a fallback and the adjuster will likely leverage Machine failure metrics produced by the MCM.

## Internals

![](./content/adjuster-controller-reconcile.svg)

The reconciler handles two paths:

**Machine join** — when a `Machine` transitions to `Running` and joins the cluster for the first time, the adjuster records the join duration and updates per-`MachineDeployment` and per-`MachineProvisionKey` statistics. It then attempts to lower the `effective-creation-timeout` annotation on the `MachineDeployment` to the maximum observed join duration across recent machines, subject to the cooldown check below.

**Machine failure** — when a `Machine` transitions to `Failed` for the first time, the adjuster records the failure. If `failCount >= machineFailureThresholdMin` and `failCount / (failCount + joinCount) >= machineFailureThresholdFraction` for the `MachineProvisionKey`, it grows the `effective-creation-timeout` annotation on all associated `MachineDeployment`s by `creationTimeoutGrowthFactor`, bounded to `creationTimeoutMax`.

**Cooldown** — before writing the annotation, the adjuster checks whether `watermarkTime − lastAdjustedAt <= currentEffectiveTimeout`. If true, machines already provisioning under the current timeout are still within their creation window, so the adjustment is skipped to avoid churn.


## Config

Adjuster can be configured via command line arguments and an adjuster configuration. See [configure adjuster](../deployment/configure.md#adjuster).

