# End to End tests

## Table Of Contents
- [End to End tests](#end-to-end-tests)
  - [Table Of Contents](#table-of-contents)
  - [DWD prober](#dwd-prober)
    - [Setup](#setup)
      - [Shooted-Seed](#shooted-seed)
      - [Secret changes](#secret-changes)
      - [Prober Config](#prober-config)
    - [End-To-End Tests](#end-to-end-tests-1)
      - [Automation tests (Used for IT)](#automation-tests-used-for-it)
      - [Non-Automation tests (Timing dependent and hard to automate)](#non-automation-tests-timing-dependent-and-hard-to-automate)


## DWD prober

### Setup 

To run these tests against a DEV cluster the following setup is required.

#### Shooted-Seed
* Create a `shooted seed` setup by following instructions at [Gardener-The Hard Way](https://pages.github.tools.sap/kubernetes/onboarding-website/setup/localsetup/hardlocalsetup/) and deploy a local shoot in the shooted seed. 
* After the shoot is deployed, `annotate` the `managed resource` for DWD prober present in the `garden` namespace of the seed worker plane with the following:
```bash
# This is required to ensure that `Dependency watchdog prober pods` are not scaled up during reconciliation of the shooted seed.
kubectl -n garden annotate managedresource dependency-watchdog-probe resources.gardener.cloud/ignore=true --overwrite
```
* Scale down the DWD prober deployment in the garden namespace (in the shooted-seed) and start a local DWD process by providing the prober config and the kubeconfig of the shooted seed as command line flags - ```bash go run ./dwd.go prober --config-path=<path to prober config yaml> --kubeconfig=<path to shooted-seed kubeconfig yaml>```. To change the log level one can additionally pass `--zap-log-level=<loglevel>` command line flag which will be picked up zap logger at the time of initialization of DWD prober.

#### Secret changes
Each DWD probe leverages an `internal-probe-endpoint` and an `external-probe-endpoint` to connect to the shoot Kube API server. Internal probe DNS record points to an `in-cluster` endpoint which is only reachable from within the cluster. For tests that are run locally by starting a DWD prober process, the internal probe endpoint will have to be changed. There are two ways to do this:

* Modify the existing secret `shoot-access-dependency-watchdog-internal-probe` present in the `shoot namespace` of the `shooted seed` cluster to use the same endpoint as the external probe.
  > NOTE: If you intend to modify the existing secret, it will be restored back to its original state after every shoot reconciliation. The duration is set to 1 hour. You need to then re-apply your changes.
* Create a new secret with the changed endpoint and ensure that the prober configuration that you supply to the locally running DWD prober process points to the new secret.
  > NOTE: If you intend to use a new secret, ensure that the name should not end with `dependency-watchdog-internal-probe` or `dependency-watchdog-external-probe` to prevent its automatic removal during reconciliation of the shoot.

For these tests, both the probes use the same target url mentioned in the dns record `$SHOOTNAME-internal`.

> **NOTE:** <br/> You can also create a new internal probe endpoint which is then reachable from the locally running DWD prober process. To do that you will have to the following:
> * Create a new `DNSRecord` containing the new shoot Kube API server endpoint. This will create a provider specific route (e.g. In case of AWS it will create a AWS-Route53 endpoint)
> * To ensure that a call to this endpoint is routed to the Kube API server of the shoot do the following:
>    * Update Istio `Gateway` resource in the shoot namespace (E.g `kubectl get gateway -n <shoot-ns> kube-apiserver -oyaml`). Add the new endpoint to `spec.servers.hosts`.
>    * Update Istio `VirtualService` resource in the shoot namespace (`k get virtualservice -n <shoot-ns> kube-apiserver -oyaml`). Add the new endpoints to `spec.hosts`, `spec.tls.match.sniHosts`
> 

#### Prober Config
You can customize different configuration values by defining your own config file. Prober config used for these tests is as follows (you can also use it as a template):
```yaml
internalKubeConfigSecretName: "shoot-access-dependency-watchdog-internal-probe"
externalKubeConfigSecretName: "shoot-access-dependency-watchdog-external-probe"
probeInterval: 20s
initialDelay: 5s
successThreshold: 1
failureThreshold: 3
internalProbeFailureBackoffDuration: 30s
backOffJitterFactor: 0.2
dependentResourceInfos:
  - ref:
      kind: "Deployment"
      name: "kube-controller-manager"
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
      name: "machine-controller-manager"
      apiVersion: "apps/v1"
    scaleUp:
      level: 2
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

### End-To-End Tests

End to End tests and their results are using the prober config as mentioned above. The order of scale up and scale down operations is controlled by the `level` defined for each resource under `scaleUp` and `scaleDown` configuration elements.
> NOTE: If you choose to have a different order of scale up/down then the evaluate the test results according to the configured order. Below results as assuming the configuration defined above and should not be used as expectations start orders different from the one defined above.

As per the above configuration scaling orders are as follows:

**Scale Up Order**: 
`CA` -> `KCM` -> `MCM`

**Scale Down Order**:
`KCM` && `MCM` (concurrently) -> `CA`

There are two types of end to end tests that were done:

1. Tests that can be automated as Integration tests
2. Tests that cannot be automated as Integration tests 
   

#### Automation tests (Used for IT)
| # | Test | Result |
| :--- | :--- | :--- |
| 1 | scale down kcm and mcm | both probes pass and are healthy, scaling up operation starts, `CA` is skipped as replica count matches the desired value, first `KCM` is scaled up and then `MCM`|
| 2 | update the internal probe secret by changing the server url so that it fails | internal probe fails, external probe is skipped and any scaling of dependent resources is not done |
| 3 | internal probe is failing, change the secret back so that it succeeds | both the probes pass and scaling up happens according to the flow (skipped if replicas already match)|
| 4 | hibernate the shoot cluster | the prober is removed when hibernation is enabled, any pending scale-up/down activities will not be done |
| 5 | wake up the cluster from hibernation | new prober is added for the cluster only after the wake up process is complete |
| 6 | update the external probe secret to use an incorrect server url so that it fails | external probe fails, after failure threshold is exceeded it becomes unhealthy and starts scaling down of resources, first `KCM` and `MCM` are brought down in parallel and then `CA` |
| 7 | external probe is unhealthy, revert back to the original external probe secret so that it succeeds | Both the probes pass, both are healthy after 1 attempt(success threshold) and scaling up begins, first `CA` is scaled up, then `KCM` and then `MCM` |
| 8 | external probe is unhealthy, revert back to the original external probe secret and change the internal probe secret to use the wrong server url | internal probe fails, external probe is skipped and no scaling of dependent resources is done |
| 9 | set annotation `dependency-watchdog.gardener.cloud/ignore-scaling: true` one by one on `CA`, `KCM`, and `MCM` deployment. Do this for both the cases - external probe success and failure | In both the cases, the expected scaling operation is performed and the resources for which annotatation is set to true, are not scaled. The other deployments are scaled as expected |
| 10 | Scale down the apiserver deployment  | Internal probe fails, external probe is skipped and no scaling of dependent resources is done |
| 11 | Remove `KCM` deployment before any scale up/down is attempted. | The scale up/down flow fails to find `KCM` deployment and all retry attempts exhaust resulting in all downstream scale operations to not occur. |
| 12 | Change the `token` in the internal kubeconfig secret to get an ignorable error from internal probe. | The previous state of the probe status is maintained and further action of doing the external probe or not is done based on that. |
| 13 | Change the `token` of the external kubeconfig secret to get an ignorable error from external probe | The previous state of the probe status is maintained and further action of scaling resources is done based on that. |
| 14 | Remove the internal probe kubeconfig secret | Internal probe is skipped as client could not be created, external probe is skipped and no scaling is done. |
| 15 | Remove the external probe kubeconfig secret | Internal probe passes, external probe is  skipped as no client could be created and no scaling is done. |
| 16 | Delete the shoot from the local garden cluster | The prober is removed when the `DeletionTimestamp` is set. |
| 17 | Create a new shoot in the local garden cluster (pending) | According to current implementation, the prober is added as soon as the creation process begins. (It should be added only after creation is complete and successful) |

#### Non-Automation tests (Timing dependent and hard to automate)

| # | Test | Result |
| :--- | :--- | :--- |
| 1 | Intent of this test to check if a resource is not scaled up in a prober run if its upstream dependencies have not been scaled up. One way to test this would be to do the following:<br/> a) set an annotation `dependency-watchdog.gardener.cloud/ignore-scaling: true` on `KCM` deployment <br/> b) scale down `KCM` and `MCM` so that before the next run of the prober both have 0 replicas <br/> c) let the probes succeed to force a scale up operation.<br/> d) just at the point when `MCM` tries to scale up remove the annotation from `KCM` deployment. | Both probes pass and are healthy. In the first run, `MCM` is not scaled because its upstream dependency (`KCM` - due to delayed removal of annotation) is not scaled. In the next run of the probe, internal and external probes succeed resulting in scaling of `KCM` and then `MCM` |
| 2 | Remove `KCM` deployment just before `MCM` tries to scaleUp | Two cases are possible - 1. If the `MCM` deployment is already matching desired replicas, then no error is recorded in this run and scaling is skipped, but if it does not then "`KCM` deployment not found" error is logged and scaling is skipped. |
| 3 | Remove `MCM` deployment just before `CA` is scaled down | Two cases are possible - 1. If the `CA` deployment is already matching desired replicas, then no error is recorded in this run and scaling is skipped, but if it does not then "`MCM` deployment not found" error is logged and scaling is skipped. |