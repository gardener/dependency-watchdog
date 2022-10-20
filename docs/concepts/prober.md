
# Prober

## Overview

Prober runs asynchronous periodic probes for every shoot cluster's control plane. 

Each probe detects the reachability of `kube-apiserver` from the worker nodes and if the `kube-apiserver` is not reachable it will scale down the dependent kubernetes deployments which are given as [configuration](/example/04-dwd-prober-configmap.yaml) to the prober. Once the connectivity to `kube-apiserver` is reestablished, the prober will then proactively scale up the deployments it had scaled down earlier. 

### Origin and Purpose

In a shoot cluster (a.k.a data plane) each node runs a `kubelet` which periodically renews its lease. Leases serve as heartbeats informing Kube Controller Manager that the node is alive. The connectivity between the `kubelet` and the `kube- apiserver` can break for different reasons and not recover in time. 

As an example, imagine for one of the large shoot cluster with several hundred nodes, there is a broken NAT gateway at the Infra-Provider layer which prevents the `kubelet` to reach `kube-apiserver`. Although everything on the workers is running healthy and the kube-apiserver is accessible from the public endpoint, but as a consequence of missing heartbeat from `kubelet` for a defined grace period the Kube Controller Manager will start to transition the node(s) to `Unknown` state. Machine Controller Manager(MCM) which also runs in the shoot control plane and watches changes to the Node state shall wait for a grace period, and if the machines still remains in the `Unknown` status, `MCM` will then start to replace the unhealthy machine(s) with new ones. 

This replacement of healthy machines due to a broken connectivity between the worker nodes and the control plane results in undesired downtimes for customer workloads that were running on these otherwise healthy nodes. It is therefore required that there be an actor which detects the connectivity loss to the `kubelet` and proactively scales down components in the shoot control namespace which could exacerbate the availability of nodes in the shoot cluster. 

### Future Scope 
Although in the current offering the Prober is tailored to handle one such use case of `kube-apiserver` connectivity, but the usage of prober can be extended to solve similar needs for other scenarios where the components involved might be different.

## Dependency Watchdog Prober in Gardener

Prober is a central component which is deployed in the `garden` namespace in the seed cluster. Control plane components for each shoot cluster are deployed in a dedicated shoot namespace within the seed cluster. 

<img src="content/prober-components.excalidraw.png">

> NOTE: If you are not familiar with what gardener components like seed, shoot then please see the [appendix](#appendix) for links.

Prober periodically probes `kube-apiserver` via two separate probes:
1.  Internal Probe: Local cluster DNS name which resolves to the ClusterIP of the `kube-apiserver`
2.  External Probe: DNS name via which the kubelet running in each node in the data plane (a.k.a shoot in gardener terminology) communicates to the `kube-apiserver` running in its control plane (a.k.a seed in gardener terminology)

## Behind the scene

For all active shoot clusters (which have not been hibernated or deleted or moved to another seed via control-plane-migration), prober will schedule a probe to run periodically. During each run of a probe it will do the following:
1. Checks if the `kube-apiserver` is reachable via local cluster DNS. This should always succeed and will fail only when the `kube-apiserver` has gone down. If the `kube-apiserver` is down then there can be no further damage to the existing shoot cluster (barring new requests to the Kube Api Server) and the probe exits.
2. Only if the probe is able to reach the `kube-apiserver` via local cluster DNS, will it attempt to reach the `kube-apiserver` via internal DNS route. This is the same DNS used by the kubelet. This is not an exact replication of how the kubelet would reach its `kube-apiserver` but it is close enough. The result of the attempt to reach the `kube-apiserver` is recorded in the probe status.
3. If a probe fails to successfully reach the `kube-apiserver` via internal DNS route `failureThreshold` times consecutively then it transitions the probe to `Failed` state.
4. If and when a probe status transitions to `Failed` it will then initiate a scale-down operation as defined in the prober configuration.
5. In subsequent runs it will keep checking if it is able to reach the `kube-apiserver` via internal DNS route. If it is able to successfully reach it `successThreshold` times consecutively as defined in the prober configuration, then it will start the scale-up operation for components defined in the configuration.

### Prober lifecycle

A reconciler is registered to listen to all events for [Cluster](https://github.com/gardener/gardener/blob/master/docs/api-reference/extensions.md#extensions.gardener.cloud/v1alpha1.Cluster) resource.

When a `Reconciler` receives a request for a `Cluster` change, it will query the extension kube-api server to get
the `Cluster` resource. 

In the following cases it will either remove an existing probe for this cluster or skip creating a new probe:
1. Cluster is marked for deletion.
2. Hibernation has been enabled for the cluster.
3. There is an ongoing seed migration for this cluster.

If none of the above conditions are true and there is no existing probe for this cluster then a new probe will be created, registered and started.

For details on transitions of a probe see [probe-state-transition](probestatus.md).

## Appendix

* [Gardener](https://github.com/gardener/gardener/blob/master/docs)
* [Reverse Cluster VPN](https://github.com/gardener/gardener/blob/master/docs/proposals/14-reversed-cluster-vpn.md)