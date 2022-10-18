# Configure Dependency Watchdog Components

## Prober

Dependency watchdog prober command takes command-line-flags which are meant fine-tune the prober. In addition a `ConfigMap` is also mounted to the prober container which provides tuning knobs for the all probes that the prober starts.

## Command line arguments

Prober can be configured via the following flags:

| Flag Name | Type | Default Value | Description |
| --- | --- | --- | --- |
| kube-api-burst | int | 10 | Burst to use while talking with kubernetes API server. The number must be >= 0. If it is 0 then a default value of 10 will be used |
| kube-api-qps | float | 5.0 | Maximum QPS (queries per second) allowed when talking with kubernetes API server. The number must be >= 0. If it is 0 then a default value of 5.0 will be used |
| concurrent-reconciles | int | 1 | Maximum number of concurrent reconciles |
| config-file | string | NA | Path of the config file containing the configuration to be used for all probes |
| metrics-bind-addr | string | ":9643" | The TCP address that the controller should bind to for serving prometheus metrics |
| health-bind-addr | string | ":9644" | The TCP address that the controller should bind to for serving health probes |
| enable-leader-election | bool | false | In case prober deployment has more than 1 replica for high availability, then it will be setup in a active-passive mode. Out of many replicas one will become the leader and the rest will be passive followers waiting to acquire leadership in case the leader dies. |
| leader-election-namespace | string | "garden" | Namespace in which leader election resource will be created. It should be the same namespace where DWD pods are deployed |
