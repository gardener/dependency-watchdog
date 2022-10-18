# Weeder

## Table of Contents
  - [What and Why](#what-and-why)
  - [Prerequisites](#prerequisites)
    - [Service](#service)
    - [Endpoints](#endpoints)
  - [Config](#config)
  - [Internals](#internals)
  - [Points to Note](#points-to-note)

## What and Why? 

Weeder is a loop which watches a k8s service for any downtime, and after the recovery of the service from the downtime, it restarts any Crashlooping pods which depends on that service.
This is helpful because kubernetes pod restarts a container with an exponential backoff when the pod is in `CrashLoopBackOff` state. This backoff could become quite large if the service stays down for long. Presence of weeder would not let that happen as it'll restart the pod.

## Prerequisites

Before we understand how Weeder works , we need to be familiar with a few K8s concepts. 
### Service

A service provides a stable IP for a set of pods serving info. Its a standard practice to have pods of a deployment behind a service. Service also takes care of load-balancing.
More info : https://kubernetes.io/docs/concepts/services-networking/service/

### Endpoints

Endpoints is a k8s resource which tracks the pod which are backing a particular k8s service. More specifically the containers which will serve traffic, the (IP,pod) tuple

`kubectl get endpoints` output:

```
kube-apiserver               10.243.134.220:443                                                       89d
kube-controller-manager      10.243.134.6:10257                                                       89d
kube-scheduler               10.243.134.13:10259                                                      89d
kube-state-metrics           10.243.130.192:8080                                                      89d
loki                         10.243.130.151:8080,10.243.130.151:9273,10.243.130.151:3100              89d
```
Endpoints controller present in kube-controller-manager keep updating this endpoint resource with IPs of the pod backing the service.

More info: https://theithollow.com/2019/02/04/kubernetes-endpoints/

There is one endpoints resource for one service resource. 

`Note`: Services can be defined without selectors in which case endpoints objects are not created. The given documentation is explained with assumption that services with label selectors are used so using them interchangably. Though they are supported because Weeder doesn't take the service resource into account, its only concerned with endpoints resource. So in the config file for weeder, only endpoints name should be specified.

## Config

There are certain config parameters which needs to be passed to weeder as a configmap. An example configmap can be found [here](../../example/04-dwd-prober-configmap.yaml)

## Internals

Weeder keeps a watch on the events for the specified endpoints in the config. For every endpoints a list of `podSelectors` can be specified. It cretes a weeder object per endpoints resource when it receives a satisfactory `Create` or `Update` event. Then for every podSelector it creates a goroutine. This goroutine keeps a watch on the pods with labels as per the podSelector and kills any pod which turn into `CrashLoopBackOff`. Goroutine live for 5minutes by default. This time is configurable using the `watchDuration` in the config.

Lets consider the diagram below

![Weeder example](content/weeder-concept-example.png)

The diagram depicts the two (service,dependent-pods) relations specified in the config file specified in the [config-file-for-weeder](#config) section. Lets consider the (etcd,KAPI) relation for now.</br>

**Note**: Time here is just for showing the series of events

* `t=0` -> both etcd pods go down (assuming 2)
* `t=10` -> KAPI starts CrashLooping
* `t=100` -> both etcd pods recover together
* `t=101` -> Weeder sees `Update` event for `etcd-main-client` endpoints resource
* `t=102` -> go routine created to keep watch on KAPI pods
* `t=103` -> KAPI pod deleted
* `t=104` -> new KAPI pod created by replica-set controller in KCM


## Points to Note

* Weeder only respond on `Update` events where a `notReady` endpoints resource turn to `Ready`. Thats why there was no weeder action at time `t=10` in the example above.
  * `notReady` -> no backing pod is Ready
  * `Ready`    -> atleast one backing pod is Ready
* Weeder doesn't respond on `Delete` events


