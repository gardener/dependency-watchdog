# End to End tests

## Table Of Contents

- [DWD prober](#dwd-prober)
    - [Setup](#setup)
        - [Shooted-Seed](#shooted-seed)
        - [Prober Config](#prober-config)
    - [End-To-End Tests](#end-to-end-tests)
- [Weeder e2e tests](#weeder-e2e-tests)

## DWD prober

### Setup

To run these tests against a DEV cluster the following setup is required.

#### Shooted-Seed
* Checkout the latest release of the [gardener](https://github.com/gardener/gardener) repository. Update the DWD image in the
  `imagevector/images.yaml` file to the DWD image you want to test.
* Create a `shooted seed` setup by following running the script
  at [Gardener-Extensions-Setup](https://github.tools.sap/kubernetes/onboarding/blob/master/setup/localsetup/hacks/local-setup-extensions.sh)
  and deploy a local shoot in the shooted seed.
* If you want to update the DWD image after the setup is done, you can do so by changing the image in the `imagevector/images.yaml`
  file in the cloned gardener repo and running `make gardener-extensions-up`.
* Another way to update the DWD image after the setup is done is to `annotate` the `managed resource` for DWD prober present in the `garden` namespace of the
  seed worker plane with the following and then updating the deployment with the new image:

```bash
# This is required to ensure that `Dependency watchdog prober pods` are not scaled up during reconciliation of the shooted seed.
kubectl -n garden annotate managedresource dependency-watchdog-probe resources.gardener.cloud/ignore=true --overwrite
```

#### Prober Config

You can customize different configuration values by defining your own config file. Prober config used for these tests is
as follows (you can also use it as a template):

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

End to End tests and their results are using the prober config as mentioned above. The order of scale up and scale down
operations is controlled by the `level` defined for each resource under `scaleUp` and `scaleDown` configuration
elements.
> NOTE: If you choose to have a different order of scale up/down then evaluate the test results according to the
> configured order. Below results as assuming the configuration defined above and should not be used as expectations
> start
> orders different from the one defined above.

As per the above configuration scaling orders are as follows:

**Scale Up Order**:
`KCM` -> `MCM` -> `CA`

**Scale Down Order**:
`MCM` && `CA` (concurrently) -> `KCM`

There are two types of end to end tests that were done:

The below tests were done with replica count in MCM deployment as 2 to check that the replica count after scale up is as
expected or not.

| #  | Test                                                                                                                                                                                                                   | Result                                                                                                                                                                                                                                                          |
|----|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1  | Create a new workerless shoot in the local garden cluster.                                                                                                                                                             | The prober is not added for the workeless shoot.                                                                                                                                                                                                                |
| 2  | Create a cluster with min=max=1 for the worker.                                                                                                                                                                        | The prober is added after shoot creation is successful.                                                                                                                                                                                                         |
| 3  | Cluster is created successfully and in a healthy state (min=max=1). Delete the node lease in kube-node-lease ns                                                                                                        | Scale out is done as no candidate leases are present.                                                                                                                                                                                                           |
| 4  | Cluster is successfully created and in a healthy state (min=max=3). Scale down kcm and mcm deployments                                                                                                                 | `MCM` and `KCM` are scaled up as per the order in the config.                                                                                                                                                                                                   |
| 5  | Cluster is successfully created and in a healthy state (min=max=3). Scale down the api-server deployment.                                                                                                              | API server probe fails, Lease probe is skipped and no scaling of dependent resources is done.                                                                                                                                                                   |
| 6  | Cluster is successfully created and in a healthy state (min=max=3). Update the API server probe secret by changing the server url so that it fails. Once the probe fails, update the secret back to its original state | API server probe fails, lease probe is skipped and scaling of dependent resources is not done. Once the secret is reverted, API server probe will succeed.                                                                                                      |
| 7  | Cluster is successfully created and in a healthy state (min=max=3). Set annotation `dependency-watchdog.gardener.cloud/ignore-scaling: true` on `MCM` deployment.                                                      | The expected scaling operation is performed and the resources for which annotation is set to true, are not scaled. The other deployments are scaled as expected                                                                                                 |
| 8  | Cluster is successfully created and in a healthy state (min=max=3). Block outbound traffic from shoot. This will cause leases to not be renewed. Later unblock the outbound traffic from the shoot.                    | Lease probe fails, dependent resources are scaled down. Once outbound traffic is unblocked Lease probe succeeds and scales up dependent resources.                                                                                                              |
| 9  | Cluster is successfully created and in a healthy state (min=max=3). Annotate two nodes with `node.machine.sapcloud.io/not-managed-by-mcm: true`.                                                                       | No scaling is done as there is only one candidate lease present (unmanaged nodes are ignored by DWD).                                                                                                                                                           |
| 10 | Cluster is successfully created and in a healthy state (min=max=3). Delete 2 machines. This will result in the cluster having `2 machines in `Terminating` State, 2 in `Pending` state and `1` in Running state.       | The prober will ignore `Terminating` machines and not perform any scaling operation as there is only 1 candidate lease in the cluster. (`Pending` machines have no registered nodes yet.)                                                                       |
| 11 | Cluster is successfully created and in a healthy state (min=max=3). Hibernate the shoot cluster. Wake up the cluster after hibernation is successful.                                                                  | The prober is removed when hibernation is enabled, scaling up of any of the needed resources is done by the hibernation logic in gardener/gardener and it successfully hibernates the shoot. New prober will be added only after cluster successfully wakes up. |
| 12 | Cluster is successfully created and in a healthy state (min=max=3). Define `MemoryPressure` nodeCondition in `machineControllerSettings` for the worker. Cause `MemoryPressure` on 2 nodes.                            | The prober will ignore nodes with `MemoryPressure` and not perform any scaling operation as there is only 1 candidate lease in the cluster.                                                                                                                     |
| 13 | Migrate shoot from one seed to another                                                                                                                                                                                 | The prober is removed from the source seed as soon as migration starts. A new prober is started in the destination seed after restore is successful.                                                                                                            |
| 14 | Delete the shoot from the local garden cluster                                                                                                                                                                         | The prober is removed when the `DeletionTimestamp` is set.                                                                                                                                                                                                      |

## Weeder e2e tests

| # | Test                                  | Result                                                                                                                                                                 |
|:--|:--------------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1 | scale down api server                 | weeder is not started as no endpoint subset would be ready                                                                                                             |
| 2 | scaled up api server after scale down | dependent pods like KCM , kube-scheduler which go in `CrashLoopBackOff` are restarted                                                                                  |
| 3 | scale down etcd                       | weeder is not started as no endpoint subset would be ready                                                                                                             |
| 4 | scale up etcd after scale down        | weeder for `etcd` endpoint started which restarted `kube-apiserver` and then weeder started for `kube-apiserver` which restarted its dependent `CrashloopBackoff` pods |
