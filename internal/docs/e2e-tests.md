# End to End tests

## Table Of Contents
  - [DWD prober](#dwd-prober)
    - [Setup](#setup)
      - [Shooted-Seed](#shooted-seed)
      - [Secret changes](#secret-changes)
      - [Prober Config](#prober-config)
    - [End-To-End Tests](#end-to-end-tests)
  - [Weeder e2e tests](#weeder-e2e-tests)


## DWD prober

### Setup 

To run these tests against a DEV cluster the following setup is required.

#### Shooted-Seed
* Create a `shooted seed` setup by following running the script at [Gardener-Extensions-Setup](https://github.tools.sap/kubernetes/onboarding/blob/master/setup/localsetup/hacks/local-setup-extensions.sh) and deploy a local shoot in the shooted seed.(Use latest gardener version if possible)
* After the shoot is deployed, `annotate` the `managed resource` for DWD prober present in the `garden` namespace of the seed worker plane with the following:
```bash
# This is required to ensure that `Dependency watchdog prober pods` are not scaled up during reconciliation of the shooted seed.
kubectl -n garden annotate managedresource dependency-watchdog-probe resources.gardener.cloud/ignore=true --overwrite
```
* Check the role/clusterrole for dependency-watchdog-prober in the `garden` namespace of the seed worker plane and add a `patch` verb for deployment/scale resource if not present.
* Check the role/rolebinding in the shoot for the dependency-watchdog-probe related service account in the `kube-system` namespace. Add rules for listing leases in the `kube-node-lease` namespace.
* Scale down the DWD prober deployment in the garden namespace (in the shooted-seed) and start a local DWD process by providing the prober config and the kubeconfig of the shooted seed as command line flags - ```bash go run ./dwd.go prober --config-file=<path to prober config yaml> --kubeconfig=<path to shooted-seed kubeconfig yaml>```. To change the log level one can additionally pass `--zap-log-level=<loglevel>` command line flag which will be picked up zap logger at the time of initialization of DWD prober.
* Another way of running DWD is to change the image of the DWD deployment at `imagevector/images.yaml` in the cloned gardener repo used for the setup. (This needs to be done after the script checks out the desired gardener version and before `make gardener-extensions-up` is run)

#### Secret changes
DWD API server probe leverages an `apiserver-probe-endpoint`(name might vary)  to connect to the shoot Kube API server. API server probe DNS record points to an `in-cluster` endpoint which is only reachable from within the cluster. For tests that are run locally by starting a DWD prober process, the api server probe endpoint will have to be changed. This can be done in the following way:

> **NOTE:** <br/> Create a new apiserver probe endpoint which is then reachable from the locally running DWD prober process. To do that you will have to the following:
> * Create a new `DNSRecord` containing the new shoot Kube API server endpoint. This will create a provider specific route (e.g. In case of AWS it will create a AWS-Route53 endpoint)
> * To ensure that a call to this endpoint is routed to the Kube API server of the shoot do the following:
    >    * Update Istio `Gateway` resource in the shoot namespace (E.g `kubectl get gateway -n <shoot-ns> kube-apiserver -oyaml`). Add the new endpoint to `spec.servers.hosts`.
>    * Update Istio `VirtualService` resource in the shoot namespace (`k get virtualservice -n <shoot-ns> kube-apiserver -oyaml`). Add the new endpoints to `spec.hosts`, `spec.tls.match.sniHosts`


* Modify the existing secret `shoot-access-dependency-watchdog-apiserver-probe`(name might vary) present in the `shoot namespace` of the `shooted seed` cluster to use the new endpoint.
  > NOTE: If you intend to modify the existing secret, it will be restored back to its original state after every shoot reconciliation. The duration is set to 1 hour. You need to then re-apply your changes.
* Create a new secret with the changed endpoint and ensure that the prober configuration that you supply to the locally running DWD prober process points to the new secret.
  > NOTE: If you intend to use a new secret, ensure that the name should not end with `dependency-watchdog-internal-probe` or `dependency-watchdog-external-probe` to prevent its automatic removal during reconciliation of the shoot.

For these tests, the API server probe uses the target url mentioned in the dns record `$SHOOTNAME-apiserver`(name might vary).

#### Prober Config
You can customize different configuration values by defining your own config file. Prober config used for these tests is as follows (you can also use it as a template):
```yaml
kubeConfigSecretName: "shoot-access-dependency-watchdog-api-server-probe" # name of the secret can vary
kcmNodeMonitorGraceDuration: 120s
dependentResourceInfos:
  - ref:
      kind: "Deployment"
      name: "kube-controller-manager"
      apiVersion: "apps/v1"
    optional: false
    scaleUp:
      level: 0
    scaleDown:
      level: 1
  - ref:
      kind: "Deployment"
      name: "machine-controller-manager"
      apiVersion: "apps/v1"
    optional: false
    scaleUp:
      level: 1
      initialDelay: 30s
    scaleDown:
      level: 0
  - ref:
      kind: "Deployment"
      name: "cluster-autoscaler"
      apiVersion: "apps/v1"
    optional: true
    scaleUp:
      level: 2
    scaleDown:
      level: 0
```

### End-To-End Tests

End to End tests and their results are using the prober config as mentioned above. The order of scale up and scale down operations is controlled by the `level` defined for each resource under `scaleUp` and `scaleDown` configuration elements.
> NOTE: If you choose to have a different order of scale up/down then evaluate the test results according to the configured order. Below results as assuming the configuration defined above and should not be used as expectations start orders different from the one defined above.

As per the above configuration scaling orders are as follows:

**Scale Up Order**: 
`KCM` -> `MCM` -> `CA`

**Scale Down Order**:
`MCM` && `CA` (concurrently) -> `KCM`

There are two types of end to end tests that were done:

1. Tests that can be automated as Integration tests
2. Tests that cannot be automated as Integration tests 

| #   | Test                                                                                                                                                                                   | Result                                                                                                                                                                                      |
|:----|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1   | scale down kcm and mcm                                                                                                                                                                 | both probes pass and are healthy, scaling up operation starts, `CA` is skipped , `MCM` and `KCM` are scaled as per the order in the config.                                                 |
| 2   | update the API server probe secret by changing the server url so that it fails                                                                                                         | API server probe fails, lease probe is skipped and any scaling of dependent resources is not done                                                                                           |
| 3   | API server probe is failing, change the secret back so that it succeeds                                                                                                                | both the probes pass and scaling up happens according to the flow (skipped if replicas have min target replicas).                                                                           |
| 4   | hibernate the shoot cluster after scale down operation has happened.                                                                                                                   | the prober is removed when hibernation is enabled, scaling up of any of the needed resources is done by the hibernation logic in gardener/gardener and it successfully hibernates the shoot |
| 5   | wake up the cluster from hibernation                                                                                                                                                   | new prober is added for the cluster only after the wake up process is successfully complete                                                                                                 |
| 6   | shut down the kubelet of some nodes to cause the lease probe to fail                                                                                                                   | Lease probe fails, starts scaling down of resources.                                                                                                                                        |
| 7   | Lease probe is failing, replace machines in `Unknown` state(or restart their kubelet) so that it succeeds                                                                              | Both the probes pass, scaling up happens successfully.                                                                                                                                      |
| 8   | Lease probe is failing, replace machines in `Unknown` state(or restart their kubelet) so that it succeeds and change the API server probe secret to use the wrong server url           | API server probe fails, Lease probe is skipped and no scaling of dependent resources is done                                                                                                |
| 9   | set annotation `dependency-watchdog.gardener.cloud/ignore-scaling: true` one by one on `CA`, `KCM`, and `MCM` deployment. Do this for both the cases - Lease probe success and failure | In both the cases, the expected scaling operation is performed and the resources for which annotation is set to true, are not scaled. The other deployments are scaled as expected          |
| 10  | Scale down the api-server deployment                                                                                                                                                   | API server probe fails, Lease probe is skipped and no scaling of dependent resources is done                                                                                                |
| 11  | Remove `KCM` deployment(mandatory resource) before any scale up/down is attempted.                                                                                                     | The scale up/down flow fails to find `KCM` deployment and all retry attempts exhaust resulting in all downstream scale operations to not occur.                                             |
| 12  | Change the `token` in the API server probe kubeconfig secret to get an error from API server probe.                                                                                    | API server probe fails, Lease probe is skipped and no scaling of dependent resources is done.                                                                                               |
| 13  | Remove the API server probe kubeconfig secret                                                                                                                                          | API server probe is skipped as client could not be created, Lease probe is skipped and no scaling is done.                                                                                  |
| 14  | Delete the shoot from the local garden cluster                                                                                                                                         | The prober is removed when the `DeletionTimestamp` is set.                                                                                                                                  |
| 15  | Create a new shoot in the local garden cluster                                                                                                                                         | The prober is added only after creation of shoot is successful.                                                                                                                             |
| 16  | Migrate shoot from one seed to another                                                                                                                                                 | prober is removed from the source seed as soon as migration starts. A new prober is started in the destination seed after restore is successful.                                            |
| 17  | Start with 2 replicas for `KCM` deployment, scale it to 1 after scale down happens successfully, and then cause a scale up                                                             | Scale up of `KCM` will be skipped.                                                                                                                                                          |

The above tests were done with replica count in some deployments as 2 to check that the replica count after scale up is as expected or not.

## Weeder e2e tests
| #   | Test                                  | Result                                                                                                                                                                 |
|:----|:--------------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1   | scale down api server                 | weeder is not started as no endpoint subset would be ready                                                                                                             |
| 2   | scaled up api server after scale down | dependent pods like KCM , kube-scheduler which go in `CrashLoopBackOff` are restarted                                                                                  |
| 3   | scale down etcd                       | weeder is not started as no endpoint subset would be ready                                                                                                             |
| 4   | scale up etcd after scale down        | weeder for `etcd` endpoint started which restarted `kube-apiserver` and then weeder started for `kube-apiserver` which restarted its dependent `CrashloopBackoff` pods |
