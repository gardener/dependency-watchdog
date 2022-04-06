package cmd

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
)

const (
	defaultConcurrentReconciles = 1
	defaultMetricsBindAddress   = ":8080"
	defaultHealthBindAddress    = ":8081"
)

var (
	Commands = []*Command{
		ProberCmd,
		WeederCmd,
	}
)

type Command struct {
	Name      string
	UsageLine string
	ShortDesc string
	LongDesc  string
	AddFlags  func(fs *flag.FlagSet)
	Run       func(ctx context.Context, args []string, logger logr.Logger) error
}

type SharedOpts struct {
	// ConfigFile is the command specific configuration file path which is typically a mounted config-map YAML file
	ConfigFile string
	// ConcurrentReconciles is the maximum number of concurrent reconciles which can be run
	ConcurrentReconciles int
	// LeaderElectionNamespace is the namespace in which leader election resource will be created
	LeaderElectionNamespace string
	// EnableLeaderElection determines to use leader election when starting the manager
	EnableLeaderElection bool
	// KubeApiBurst is the maximum burst over the QPS
	KubeApiBurst int
	// KubeApiQps indicates the maximum QPS to the API server from this client
	KubeApiQps float64
	// MetricsBindAddress is the TCP address that the controller should bind to for serving prometheus metrics
	MetricsBindAddress string
	// HealthBindAddress is the TCP address that the controller should bind to for serving health probes
	HealthBindAddress string
}

func SetSharedOpts(fs *flag.FlagSet, opts *SharedOpts) {
	fs.StringVar(&opts.ConfigFile, "config-file", "", "Path of the config file containing the configuration")
	fs.IntVar(&opts.ConcurrentReconciles, "concurrent-reconciles", defaultConcurrentReconciles, "Maximum number of concurrent reconciles")
	fs.StringVar(&opts.LeaderElectionNamespace, "leader-election-namespace", "garden", "Namespace in which leader election resource will be created. It should be the same namespace where DWD pods are deployed")
	fs.BoolVar(&opts.EnableLeaderElection, "enable-leader-election", false, "Determines if leader election should be used when starting the manager")
	fs.IntVar(&opts.KubeApiBurst, "kube-api-burst", rest.DefaultBurst, "Maximum burst to throttle the calls to the API server.")
	fs.Float64Var(&opts.KubeApiQps, "kube-api-qps", float64(rest.DefaultQPS), "Maximum QPS (queries per second) allowed from the client to the API server")
	fs.StringVar(&opts.MetricsBindAddress, "metrics-bind-addr", defaultMetricsBindAddress, "The TCP address that the controller should bind to for serving prometheus metrics")
	fs.StringVar(&opts.HealthBindAddress, "health-bind-addr", defaultHealthBindAddress, "The TCP address that the controller should bind to for serving health probes")
}
