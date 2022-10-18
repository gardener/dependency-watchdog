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


## Probe Configuration

A probe configuration is mounted as `ConfigMap` to the prober container. The path to the config file is configured via `config-file` command line argument as mentioned above. Prober will start one probe per Shoot control plane hosted within the Seed cluster. Each such probe will run asynchronously and will periodically connect to the Kube ApiServer of the Shoot. Configuration below will influence each such probe.

You can view the example YAML configuration provided as `data` in a `ConfigMap` [here](../../example/03-dwd-prober-configmap.yaml).

| Name | Type |  Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| internalKubeConfigSecretName | string | Yes | NA | Name of the kubernetes Secret which has the encoded KubeConfig required to connect to the Shoot control plane Kube ApiServer via an internal domain. This typically uses the local cluster DNS. |
| externalKubeConfigSecretName | string | Yes | NA | Name of the kubernetes Secret which has the encoded KubeConfig required to connect to the Shoot control plane Kube ApiServer via an external domain. This typically uses the provider cluster DNS also used by the Kubelet running in the node of a Shoot cluster. |
| probeInterval | metav1.Duration | No |  | Interval with which each probe will run |
| initialDelay | metav1.Duration | No |  | Initial delay for the probe to become active. Only applicable when the probe is created for the first time. |
| probeTimeout | metav1.Duration | No |  | In each run of the probe it will attempt to connect to the Shoot Kube ApiServer. probeTimeout defines the timeout after which a single run of the probe will fail. |
| successThreshold | int | No | 1 | Number of consecutive times a probe successfully connects to the Shoot Kube ApiServer and gets a response from it to ascertain that the probe is healthy. |
| failureThreshold | int | No | 3 | Number of consecutive times a probe fails to get a response from the Kube ApiServer to ascertain that the probe is unheathy. |
| internalProbeFailureBackoffDuration | metav1.Duration | No |  | Only applicable for internal probe. It is the duration that a probe should backOff in case the internal probe is unhealthy before re-attempting. This prevents too many calls to the Kube ApiServer. |
| backoffJitterFactor | float64 | No | Jitter with which a probe is run |
| dependentResourceInfos | []prober.DependentResourceInfo | Yes | If a `failureThreshold` is breached by a probe while trying to reach the Shoot Kube ApiServer via external DNS route then it scales down the dependent resources defined by this property. Similarly if the probe is now able to reach the Shoot Kube ApiServer it transitions the probe status from unhealthy to healthy and it scales up the dependent resources defined by this property. |


### DependentResourceInfo

### ScaleInfo

