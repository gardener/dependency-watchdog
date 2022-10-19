# Configure Dependency Watchdog Components

## Prober

Dependency watchdog prober command takes command-line-flags which are meant fine-tune the prober. In addition a `ConfigMap` is also mounted to the prober container which provides tuning knobs for the all probes that the prober starts.

## Command line arguments

Prober can be configured via the following flags:

| Flag Name | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| kube-api-burst | int | No | 10 | Burst to use while talking with kubernetes API server. The number must be >= 0. If it is 0 then a default value of 10 will be used |
| kube-api-qps | float | No | 5.0 | Maximum QPS (queries per second) allowed when talking with kubernetes API server. The number must be >= 0. If it is 0 then a default value of 5.0 will be used |
| concurrent-reconciles | int | No | 1 | Maximum number of concurrent reconciles |
| config-file | string | Yes | NA | Path of the config file containing the configuration to be used for all probes |
| metrics-bind-addr | string | No | ":9643" | The TCP address that the controller should bind to for serving prometheus metrics |
| health-bind-addr | string | No | ":9644" | The TCP address that the controller should bind to for serving health probes |
| enable-leader-election | bool | No | false | In case prober deployment has more than 1 replica for high availability, then it will be setup in a active-passive mode. Out of many replicas one will become the leader and the rest will be passive followers waiting to acquire leadership in case the leader dies. |
| leader-election-namespace | string | No | "garden" | Namespace in which leader election resource will be created. It should be the same namespace where DWD pods are deployed |
| leader-elect-lease-duration | time.Duration | No | 15s | The duration that non-leader candidates will wait after observing a leadership renewal until attempting to acquire leadership of a led but unrenewed leader slot. This is effectively the maximum duration that a leader can be stopped before it is replaced by another candidate. This is only applicable if leader election is enabled. |
| leader-elect-renew-deadline | time.Duration | No | 10s | The interval between attempts by the acting master to renew a leadership slot before it stops leading. This must be less than or equal to the lease duration. This is only applicable if leader election is enabled. |
| leader-elect-retry-period | time.Duration | No | 2s | The duration the clients should wait between attempting acquisition and renewal of a leadership. This is only applicable if leader election is enabled. |

You can view an example kubernetes prober [deployment](../../example/01-dwd-prober-deployment.yaml) YAML to see how these command line args are configured.


## Probe Configuration

A probe configuration is mounted as `ConfigMap` to the prober container. The path to the config file is configured via `config-file` command line argument as mentioned above. Prober will start one probe per Shoot control plane hosted within the Seed cluster. Each such probe will run asynchronously and will periodically connect to the Kube ApiServer of the Shoot. Configuration below will influence each such probe.

You can view an example YAML configuration provided as `data` in a `ConfigMap` [here](../../example/04-dwd-prober-configmap.yaml).

| Name | Type |  Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| internalKubeConfigSecretName | string | Yes | NA | Name of the kubernetes Secret which has the encoded KubeConfig required to connect to the Shoot control plane Kube ApiServer via an internal domain. This typically uses the local cluster DNS. |
| externalKubeConfigSecretName | string | Yes | NA | Name of the kubernetes Secret which has the encoded KubeConfig required to connect to the Shoot control plane Kube ApiServer via an external domain. This typically uses the provider cluster DNS also used by the Kubelet running in the node of a Shoot cluster. |
| probeInterval | metav1.Duration | No | 10s | Interval with which each probe will run. |
| initialDelay | metav1.Duration | No | 30s | Initial delay for the probe to become active. Only applicable when the probe is created for the first time. |
| probeTimeout | metav1.Duration | No | 30s | In each run of the probe it will attempt to connect to the Shoot Kube ApiServer. probeTimeout defines the timeout after which a single run of the probe will fail. |
| successThreshold | int | No | 1 | Number of consecutive times a probe successfully connects to the Shoot Kube ApiServer and gets a response from it to ascertain that the probe is healthy. |
| failureThreshold | int | No | 3 | Number of consecutive times a probe fails to get a response from the Kube ApiServer to ascertain that the probe is unheathy. |
| internalProbeFailureBackoffDuration | metav1.Duration | No | 30s | Only applicable for internal probe. It is the duration that a probe should backOff in case the internal probe is unhealthy before re-attempting. This prevents too many calls to the Kube ApiServer. |
| backoffJitterFactor | float64 | No | 0.2 | Jitter with which a probe is run. |
| dependentResourceInfos | []prober.DependentResourceInfo | Yes | NA | Detailed below. |


### DependentResourceInfo

If a `failureThreshold` is breached by a probe while trying to reach the Shoot Kube ApiServer via external DNS route then it scales down the dependent resources defined by this property. Similarly if the probe is now able to reach the Shoot Kube ApiServer it transitions the probe status from unhealthy to healthy and it scales up the dependent resources defined by this property.

Each dependent resource info has the following properties:

| Name | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| ref | autoscalingv1.CrossVersionObjectReference | Yes | NA | It is a collection of ApiVersion, Kind and Name for a kubernetes resource thus serving as an identifier. |
| shouldExist | bool | Yes | NA | It is possible that a dependent resource is optional for a Shoot control plane. This property enables a probe to determine the correct behavior in case it is unable to find the resource identified via `ref`. |
| scaleUp | prober.ScaleInfo | No | | Captures the configuration to scale up this resource. Detailed below. |
| scaleDown | prober.ScaleInfo | No | | Captures the configuration to scale down this resource. Detailed below. |


### ScaleInfo

How to scale a `DependentResourceInfo` is captured in `ScaleInfo`. It has the following properties:

| Name | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| level | int | Yes | NA | Detailed below. |
| initialDelay | metav1.Duration | No | 0s (No initial delay) | Once a decision is taken to scale a resource then via this property a delay can be induced before triggering the scale of the dependent resource. |
| timeout | metav1.Duration | No | 30s | Defines the timeout for the scale operation to finish for a dependent resource. |
| replicas | int | Yes | NA | It is the desired set of replicas post scale up/down operation for a dependent resource. |

**Level**

Each dependent resource that should be scaled up or down is associated to a level. Levels are ordered and processed in ascending order (starting with 0 assigning it the highest priority). Consider the following configuration:

```yaml
dependentResourceInfos:
  - ref: 
      kind: "Deployment"
      name: "kube-controller-manager"
      apiVersion: "apps/v1"
    scaleUp: 
      level: 1 
      replicas: 1 
    scaleDown: 
      level: 0 
      replicas: 0 
  - ref:
      kind: "Deployment"
      name: "machine-controller-manager"
      apiVersion: "apps/v1"
    scaleUp:
      level: 1
      replicas: 1
    scaleDown:
      level: 1
      replicas: 0
  - ref:
      kind: "Deployment"
      name: "cluster-autoscaler"
      apiVersion: "apps/v1"
    scaleUp:
      level: 0
      replicas: 1
    scaleDown:
      level: 2
      replicas: 0
```
Let us order the dependent resources by their respective levels for both scale-up and scale-down. We get the following order:

_Scale Up Operation_

Order of scale up will be:
1. cluster-autoscaler
2. kube-controller-manager and machine-controller-manager will be scaled up concurrently after cluster-autoscaler has been scaled up.

_Scale Down Operation_

Order of scale down will be:
1. kube-controller-manager
2. machine-controller-manager after (1) has been scaled down.
3. cluster-autoscaler after (2) has been scaled down.


## Weeder