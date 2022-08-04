package cmd

import (
	"context"
	"flag"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
)

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
	--kubeconfig
		Path to the kubeconfig file. If not specified, then it will default to the service account token to connect to the kube-api-server	
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
`,
		AddFlags: addWeederFlags,
		Run:      startWeederControllerMgr,
	}
	weederOpts = weederOptions{}
)

type weederOptions struct {
	SharedOpts
}

func addWeederFlags(fs *flag.FlagSet) {
	SetSharedOpts(fs, &weederOpts.SharedOpts)
}

func startWeederControllerMgr(ctx context.Context, args []string, logger logr.Logger) (manager.Manager, error) {
	return nil, nil
}
