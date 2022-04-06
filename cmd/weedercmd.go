package cmd

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
)

const defaultWatchDuration = "2m"

var (
	WeederCmd = &Command{
		Name:      "weeder",
		UsageLine: "",
		ShortDesc: "",
		LongDesc: `Watches for Kubernetes endpoints for a service. If the endpoints transition from being
unavailable to now being available, it checks all dependent pods for CrashLoopBackOff condition. It attempts to
restore these dependent pods by deleting them so that they are started again by respective controller. In essence
it weeds out the bad pods.

Flags:
	--config-file
		Path of the configuration file containing probe configuration and scaling controller-reference information
	--concurrent-reconciles
		Maximum number of concurrent reconciles which can be run. <optional>
	--leader-election-namespace
		Namespace in which leader election namespace will be created. This is typically the same namespace where DWD controllers are deployed.
	--enable-leader-election
		Determines if the leader election needs to be enabled.
	--kube-api-qps
		Maximum QPS to the API server from this client.
	--kube-api-burst
		Maximum burst over the QPS
	--metrics-bind-address
		TCP address that the controller should bind to for serving prometheus metrics
	--health-bind-address
		TCP address that the controller should bind to for serving health probes
	--watch-duration
		Duration for which all dependent pods for a service under surveillance will be watched after the service has recovered. 
		If the dependent pods have not transitioned to CrashLoopBackOff in this duration then it is assumed that they will not enter that state.
`,
		AddFlags: addWeederFlags,
		Run:      startWeederControllerMgr,
	}
	weederOpts = weederOptions{}
)

type weederOptions struct {
	watchDuration string
	SharedOpts
}

func addWeederFlags(fs *flag.FlagSet) {
	fs.StringVar(&weederOpts.watchDuration, "watch-duration", defaultWatchDuration, "max duration to watch for all dependent pods")
	SetSharedOpts(fs, &weederOpts.SharedOpts)
}

func startWeederControllerMgr(ctx context.Context, args []string, logger logr.Logger) error {
	return nil
}
