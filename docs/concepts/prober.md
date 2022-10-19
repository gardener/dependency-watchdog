
# Prober

## Overview

Prober starts asynchronous and periodic probes for every shoot cluster's control plane. Each probe detects the reachability status for the Kube ApiServer for each shoot cluster and based on the status it takes decision to scale up/down dependent kubernetes deployments which are given as configuration to the prober.

### Why is this required?

In a shoot cluster (a.k.a data plane) each node runs a kubelet which periodically renewes its lease. Leases serve as heartbeats informing Kube Controller Manager that the node is alive. The connectivity between the kubelet and the Kube ApiServer can break for different reasons and not recover in time. This happend for one of the large shoot cluster with several hundred nodes where the issue was with a NAT gateway on the shoot cluster which prevented the Kubelet to reach its control plane Kube ApiServer. As a consequence, Kube Controller Manager transitioned the nodes to `Unknown` state. 

There is another component namely Machine Controller Manager which also runs in the shoot control plane which reacts to any changes to the Node status and then takes action to recover backing VMs/machine(s). It waits for a timeout and then starts to bring up new machines and deletes the older machines. This has a potential for causing downtime for customer workloads that were running on existing nodes. It is therefore required that there be an actor which detects the connectivity loss to the Kubelet and proactively scales down components in the shoot control namespace which could exacerbate the availability of nodes in the shoot cluster. 

However, this is one such use case and usage of prober can be extended to other similar scenarios where the components involved might be different.

## Dependency Watchdog Prober in Gardener

Prober is a central component which is set up in the `garden` namespace in the seed cluster. Control plane components for a shoot are deployed in a dedicated control-namespace for the shoot within the seed cluster. 

<img src="content/prober-components.excalidraw.png">

> NOTE: If you are not familiar with what gardener components like seed, shoot then please see the [appendix](#appendix) for links.

Prober periodically probes Kube ApiServer via two separate probes:
1.  Internal Probe: Local cluster DNS name which resolves to the ClusterIP of the Kube Apiserver
2.  External Probe: DNS name via which the kubelet running in each node in the data plane (a.k.a shoot in gardener terminology) communicates to the Kube Apiserver running in its control plane (a.k.a seed in gardener terminology)

## Behind the scene

For all live shoot clusters (which have not been hibernated or deleted or moved to another seed via control-plane-migration), prober will schedule a probe to run periodically. In a single run of a probe it will do the following:
1. Tries to determine if the Kube ApiServer is reachable via local cluster DNS. This should always succeed and will fail only when the Kube ApiServer has gone down. If the Kube ApiServer is down then there can be no further damage to the existing shoot cluster (barring new requests to the Kube Api Server).
2. Only if the probe is able to reach the Kube ApiServer via local cluster DNS, will it attempt to reach the Kube ApiServer via internal DNS route. This is the same DNS used by the kubelet. This is not an exact replication of how the kubelet would reach its Kube ApiServer but it is close enough. The result of the attempt to reach the Kube ApiServer is recorded in the probe status.
3. If a probe fails to successfully reach the Kube ApiServer via interal DNS route `failureThreshold` times consecutively then it transitions the probe to `Failed` state.
4. If and when a probe status transitions to `Failed` then it will initiate a scale-down operation as defined in the prober configuration.
5. In subsequent runs it will keep checking if it is able to reach the Kube ApiServer via internal DNS route. If it is able to successfully reach it `successThreshold` times consecutively as defined in the prober configuration, then it will start the scale-up operation for components defined in the configuration.

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