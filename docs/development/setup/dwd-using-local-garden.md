# Dependency Watchdog with Local Garden Cluster

## Setting up Local Garden cluster

A convenient way to test local dependency-watchdog changes is to use a local garden cluster.
To setup a local garden cluster you can follow the [setup-guide](https://github.com/gardener/gardener/blob/master/docs/deployment/getting_started_locally.md).

## Dependency Watchdog resources

As part of the local garden installation, a `local` seed will be available. 

### Dependency Watchdog resources created in the seed

#### Namespaced resources
In the `garden` namespace of the seed cluster, following resources will be created:

| Resource (GVK) | Name |
| ---- | ---- |
| {apiVersion: v1, Kind: ServiceAccount} | dependency-watchdog-prober |
| {apiVersion: v1, Kind: ServiceAccount} | dependency-watchdog-weeder |
| {apiVersion: apps/v1, Kind: Deployment} | dependency-watchdog-prober |
| {apiVersion: apps/v1, Kind: Deployment} | dependency-watchdog-weeder |
| {apiVersion: v1, Kind: ConfigMap} | dependency-watchdog-prober-* |
| {apiVersion: v1, Kind: ConfigMap} | dependency-watchdog-weeder-* |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: Role} | gardener.cloud:dependency-watchdog-prober:role |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: Role} | gardener.cloud:dependency-watchdog-weeder:role |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: RoleBinding} | gardener.cloud:dependency-watchdog-prober:role-binding |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: RoleBinding} | gardener.cloud:dependency-watchdog-weeder:role-binding |
| {apiVersion: resources.gardener.cloud/v1alpha1, Kind: ManagedResource} | dependency-watchdog-prober |
| {apiVersion: resources.gardener.cloud/v1alpha1, Kind: ManagedResource} | dependency-watchdog-weeder |
| {apiVersion: v1, Kind: Secret} | managedresource-dependency-watchdog-weeder |
| {apiVersion: v1, Kind: Secret} | managedresource-dependency-watchdog-prober |

#### Cluster resources

| Resource (GVK) | Name |
| ---- | ---- |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: ClusterRole} | gardener.cloud:dependency-watchdog-prober:cluster-role |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: ClusterRole} | gardener.cloud:dependency-watchdog-weeder:cluster-role |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: ClusterRoleBinding} | gardener.cloud:dependency-watchdog-prober:cluster-role-binding |
| {apiVersion: rbac.authorization.k8s.io/v1, Kind: ClusterRoleBinding} | gardener.cloud:dependency-watchdog-weeder:cluster-role-binding |

### Dependency Watchdog resources created in Shoot control namespace


| Resource (GVK) | Name |
| ---- | ---- |
| {apiVersion: v1, Kind: Secret} | shoot-access-dependency-watchdog-external-probe |
| {apiVersion: v1, Kind: Secret} | shoot-access-dependency-watchdog-internal-probe |