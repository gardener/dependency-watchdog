
# Weeder scratch notes
## TODOs
* Check if the `LastOperation` is `Restore` then should we also check if the op has succeeded before we start a probe?
* Change the name of role, rolebinding, clusterrole and clusterrolebinding that exists today for previous `endpoint`
* In prober and weeder check if command line flag `--kube-config` is required and how do you handle the case of token expiry which is today implemented via projected service account token with a set expiry duration.

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

### Validations
* There is at least one service
* For each service there is at least one dependant specified
* For each dependant validate the LabelSelector defined is of the correct specification

All validations should be done as part of `loading the configuration` and if there is an error then exit early with an unrecoverable error.
Once the configuration struct is populated and passed downstream then it should be assumed that it is entirely correct.

## Controller

Create a reconciler/controller which will get `CREATE/UPDATE` events for endpoints. While registering the controller with
the manager, add a predicate which will filter out the events which do not match the service names that are configured in
the weeder ConfigMap.

## Weeder

A weeder watches for dependant pods that are defined for a service and will optionally `weed out` 
the ones which are in `CrashLoopBackOff`. A weeder is created per endpoint. It is possible that there are more than
one `LabelSelector`'s defined for an endpoint. Weeder internally will start one go-routine per `LabelSelector` - 
this introduces an un-checked level of concurrency and ideally one typically solves this issue by having 
a fixed size worker-pool per weeder. But this would be pre-mature optimization as the current concurrency level
is quite low owing to the fact that we just have 1 `matchExpression` defined per service.

## Weeder Manager

A weeder manager maintains one weeder per endpoint key which represents an event
for a service for which a weeder instance has been created.
It also has a `janitor` which periodically checks if any weeder `isClosed` and will 
remove such weeders from the map that it will internally maintain.

```go
type Registrar interface {
	// Checks if there is an existing registration, if there is then
	// it will close the existing Weeder, removes it from the map
	// and then registers the new weeder and returns the regKey
    Register(Weeder w) string
	Unregister(regKey string)
}
```

## Weeder

For every service endpoint a weeder is created which will be responsible
for the following:
* Gets all the `LabelSelector`'s defined for this service from the config
* For each of these it starts a go routine a.k.a `Watcher` which will watch
