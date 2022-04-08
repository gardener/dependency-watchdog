package cmd

import (
	"context"
	"flag"
	"fmt"
	"github.com/gardener/dependency-watchdog/controllers"
	"github.com/gardener/dependency-watchdog/internal/prober"
	gardenextensions "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	proberLeaderElectionID = "dwd-prober-leader-election"
)

var (
	ProberCmd = &Command{
		Name:      "prober",
		UsageLine: "",
		ShortDesc: "Probes Kubernetes API and Scales Up/Down dependent resources based on its reachability",
		LongDesc: `For each shoot cluster it will start a probe which periodically probes the API server via an internal and an external endpoint. 
If the API server continues to be un-reachable beyond a threshold then it scales down the dependent controllers. Once the API 
server is again reachable then it will restore by scaling up the dependent controllers.

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
`,
		AddFlags: addProbeFlags,
		Run:      startProberControllerMgr,
	}
	opts   = proberOptions{}
	scheme = runtime.NewScheme()
)

type proberOptions struct {
	SharedOpts
}

func init() {
	localSchemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		gardenextensions.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(scheme))
}

func addProbeFlags(fs *flag.FlagSet) {
	SetSharedOpts(fs, &opts.SharedOpts)
}

func startProberControllerMgr(ctx context.Context, args []string, logger logr.Logger) (manager.Manager, error) {
	proberConfig, err := prober.ReadAndUnmarshal(opts.SharedOpts.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse prober config file %s : %w", opts.SharedOpts.ConfigFile, err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         opts.SharedOpts.MetricsBindAddress,
		HealthProbeBindAddress:     opts.SharedOpts.HealthBindAddress,
		LeaderElection:             opts.SharedOpts.EnableLeaderElection,
		LeaderElectionID:           proberLeaderElectionID,
		LeaderElectionNamespace:    opts.SharedOpts.LeaderElectionNamespace,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start the prober controller manager %w", err)
	}

	scalesGetter, err := createScalesGetter(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, err
	}
	if err := (&controllers.ClusterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ScaleGetter: scalesGetter,
		ProberMgr:   prober.NewManager(),
		ProbeConfig: proberConfig,
	}).SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to register cluster reconciler with the prober controller manager %w", err)
	}
	return mgr, nil
}

func createScalesGetter(config *rest.Config) (scale.ScalesGetter, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientSet for scalesGetter %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(clientSet.Discovery()))
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(clientSet.Discovery())
	scalesGetter := scale.New(clientSet.RESTClient(), mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	return scalesGetter, nil
}
