# Weeder

## Overview

Weeder watches for updation to service endpoints and on receipt of such an event it will create a time-bound watch for all configured dependent pods that needs to be actively recovered in case they have not yet recovered from `CrashLoopBackoff` state. In a nutshell it accelerates recovery of pods when a upstream service recovers.

An interference in automatic recovery for dependent pods is required because kubernetes pod restarts a container with an exponential backoff when the pod is in `CrashLoopBackOff` state. This backoff could become quite large if the service stays down for long. Presence of weeder would not let that happen as it'll restart the pod.

## Prerequisites

Before we understand how Weeder works, we need to be familiar with kubernetes [services & endpoints](https://kubernetes.io/docs/concepts/services-networking/service/).

> NOTE: If a kubernetes service is created with selectors then kubernetes will create corresponding endpoint resource which will have the same name as that of the service. In weeder implementation service and endpoint name is used interchangeably.

## Config

Weeder can be configured via command line arguments and a weeder configuration. See [configure weeder](../deployment/configure.md#weeder).

## Internals

Weeder keeps a watch on the events for the specified endpoints in the config. For every endpoints a list of `podSelectors` can be specified. It cretes a weeder object per endpoints resource when it receives a satisfactory `Create` or `Update` event. Then for every podSelector it creates a goroutine. This goroutine keeps a watch on the pods with labels as per the podSelector and kills any pod which turn into `CrashLoopBackOff`. Each weeder lives for `watchDuration` interval which has a default value of 5 mins if not explicitly set.

To understand the actions taken by the weeder lets use the following diagram as a reference.
<img src="content/weeder-components.excalidraw.png">
Let us also assume the following configuration for the weeder:

```yaml
watchDuration: 2m0s
servicesAndDependantSelectors:
  etcd-main-client: # name of the service/endpoint for etcd statefulset that weeder will receive events for.
    podSelectors: # all pods matching the label selector are direct dependencies for etcd service
      - matchExpressions:
          - key: gardener.cloud/role
            operator: In
            values:
              - controlplane
          - key: role
            operator: In
            values:
              - apiserver
  kube-apiserver: # name of the service/endpoint for kube-api-server pods that weeder will receive events for. 
    podSelectors: # all pods matching the label selector are direct dependencies for kube-api-server service
      - matchExpressions:
          - key: gardener.cloud/role
            operator: In
            values:
              - controlplane
          - key: role
            operator: NotIn
            values:
              - main
              - apiserver
```
Only for the sake of demonstration lets pick the first service -> dependent pods tuple (`etcd-main-client` as the service endpoint).

> 1. Assume that there are 3 replicas for etcd statefulset.
> 2. Time here is just for showing the series of events

* `t=0` -> all etcd pods go down 
* `t=10` -> kube-api-server pods transition to CrashLoopBackOff
* `t=100` -> all etcd pods recover together
* `t=101` -> Weeder sees `Update` event for `etcd-main-client` endpoints resource
* `t=102` -> go routine created to keep watch on kube-api-server pods
* `t=103` -> Since kube-api-server pods are still in CrashLoopBackOff, weeder deletes the pods to accelerate the recovery.
* `t=104` -> new kube-api-server pod created by replica-set controller in kube-controller-manager

### Points to Note

* Weeder only respond on `Update` events where a `notReady` endpoints resource turn to `Ready`. Thats why there was no weeder action at time `t=10` in the example above.
  * `notReady` -> no backing pod is Ready
  * `Ready`    -> atleast one backing pod is Ready
* Weeder doesn't respond on `Delete` events
* Weeder will always wait for the entire `watchDuration`. If the dependent pods transition to CrashLoopBackOff after the watch duration or even after repeated deletion of these pods they do not recover then weeder will exit. Quality of service offered via a weeder is only Best-Effort.


