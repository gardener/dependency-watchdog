# controlplane-restarter
This controller checks the readiness status of a service and restarts control plane components which are in a state of crashloop-backoff over an extensive period of time once the service transitions to a `Ready` state.
