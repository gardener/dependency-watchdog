package cmd

import (
	"flag"
	"fmt"
	"github.com/gardener/dependency-watchdog/controllers"
	internalutils "github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/dependency-watchdog/internal/weeder"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
)

var (
	WeederCmd = &Command{
		Name:      "weeder",
		UsageLine: "",
		ShortDesc: "Restarts CrashLooping pods which are dependant on a service , for quick availability",
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
	--leader-elect-renew-deadline
		Interval between attempts by the acting master to renew a leadership slot
	--leader-elect-retry-period
		The duration the clients should wait between attempting acquisition and renewal
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

func startWeederControllerMgr(logger logr.Logger) (manager.Manager, error) {
	weederLogger := logger.WithName("endpoints-controller")
	weederConfig, err := weeder.LoadConfig(weederOpts.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse weeder config file %s : %w", weederOpts.ConfigFile, err)
	}

	restConf := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         weederOpts.SharedOpts.MetricsBindAddress,
		HealthProbeBindAddress:     weederOpts.SharedOpts.HealthBindAddress,
		LeaderElection:             weederOpts.SharedOpts.LeaderElection.Enable,
		LeaseDuration:              &weederOpts.SharedOpts.LeaderElection.LeaseDuration,
		RenewDeadline:              &weederOpts.SharedOpts.LeaderElection.RenewDeadline,
		RetryPeriod:                &weederOpts.SharedOpts.LeaderElection.RetryPeriod,
		LeaderElectionNamespace:    weederOpts.SharedOpts.LeaderElection.Namespace,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           weederLeaderElectionID,
		Logger:                     weederLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start the weeder controller manager %w", err)
	}

	// create clientSet
	clientSet, err := internalutils.CreateClientSetFromRestConfig(restConf)
	if err != nil {
		return nil, fmt.Errorf("failed creating clientset for dwd-weeder %w", err)
	}

	if err := (&controllers.EndpointReconciler{
		Client:       mgr.GetClient(),
		SeedClient:   clientSet,
		WeederConfig: weederConfig,
		WeederMgr:    weeder.NewManager(),
	}).SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to register endpoint reconciler with weeder controller manager %w", err)
	}
	return nil, nil
}
