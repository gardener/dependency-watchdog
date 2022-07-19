

## TODOs
* Check if the `LastOperation` is `Restore` then should we also check if the op has succeeded before we start a probe?
* Rename `Config` in prober API to `ProberConfig`

## Cmd

* Add `--kubeconfig` -  Path to the kubeconfig file. If not specified, then it will default to the service account token to connect to the kube-api-server
* `deployed-namespace` - there is no need to make this configurable. It can be set to `garden` namespace in the deployment yaml
* LowPrio: Check if we really require the following leader election configuration to be set while creating the manager:
```
    // LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership. This is measured against time of
	// last observed ack. Default is 15 seconds.
	LeaseDuration *time.Duration
	// RenewDeadline is the duration that the acting controlplane will retry
	// refreshing leadership before giving up. Default is 10 seconds.
	RenewDeadline *time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions. Default is 2 seconds.
	RetryPeriod *time.Duration
 ```

## API

create a new package `weeder` in `api` and there you can have `types.go`

### Configuration spec

```yaml
servicesToDependantSelectors:
 api-server:
  podSelectors:
  - matchExpressions:
    - key: gardener.cloud/role
      operator: In
      values:
      - controlplane
    - key: role
      operator: In
      values:
      - apiserver
  - matchExpressions:
    - key: gardener.cloud/role
      operator: In
      values:
      - kube-system
  himanshuSelectors:
  - matchExpressions:
    - key: gardener.cloud/role
      operator: In
      values:
      - controlplane
```
    
```go
type WeederConfig struct {
    ServicesAndDependantSelectors map[string]DependantSelectors `yaml:"servicesAndDependantSelectors`
}

type DependantSelectors struct {
    PodSelectors []*metav1.LabelSelector `yaml:"podSelectors`
}
```
Having `DependantSelectors` struct allows to have other than `PodSelectors` in any future extensions

> NOTE: Ensure that the configuration is read, validated, default values are populated and only then create the manager. Use the configured `WeederConfig` to set the appropriate `Predicates` where you filter the endpoint events based on configured service names.


key = e1
ContextCh <- ContextMsg {e1, cancel-e1}
Map => {e1, cancel-e1}

key = e1
ContextCh <- ContextMsg {e1, cancel-e11}
Map => {e1, cancel-e11}
cancel-e1 - is called

Map => {}
cancel-e11 - is called

