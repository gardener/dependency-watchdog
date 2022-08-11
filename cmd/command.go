package cmd

import (
	"context"
	"flag"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
)

const (
	defaultConcurrentReconciles = 1
	defaultMetricsBindAddress   = ":8080"
	defaultHealthBindAddress    = ":8081"
	defaultLeaseDuration        = 15 * time.Second
	defaultRenewDeadline        = 10 * time.Second
	defaultRetryPeriod          = 2 * time.Second
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
	Run       func(ctx context.Context, args []string, logger logr.Logger) (manager.Manager, error)
}

// SharedOpts are the flags which bother prober and weeder have in common
type SharedOpts struct {
	// ConfigPath is the command specific configuration file path which is typically a mounted config-map YAML file
	ConfigPath string
	// ConcurrentReconciles is the maximum number of concurrent reconciles which can be run
	ConcurrentReconciles int
	// leaderElection defines the configuration of leader election client.
	LeaderElection LeaderElectionOpts
	// KubeApiBurst is the maximum burst over the QPS
	KubeApiBurst int
	// KubeApiQps indicates the maximum QPS to the API server from this client
	KubeApiQps float64
	// MetricsBindAddress is the TCP address that the controller should bind to for serving prometheus metrics
	MetricsBindAddress string
	// HealthBindAddress is the TCP address that the controller should bind to for serving health probes
	HealthBindAddress string
}

// LeaderElectionOpts defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionOpts struct {
	// Enable enables a leader election client to gain leadership
	// before executing the main loop. Enable this when running replicated
	// components for high availability. By default, it is false
	Enable bool
	// Namespace is the namespace in which leader election resource will be created
	Namespace string
	// LeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of a leader but un-renewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration time.Duration
	// RenewDeadline is the interval between attempts by the acting leader to
	// renew a leadership slot before it stops leading. This must be less
	// than or equal to the lease duration. This is only applicable if leader
	// election is enabled.
	RenewDeadline time.Duration
	// RetryPeriod is the duration the clients should wait between attempting
	// acquisition and renewal of a leadership. This is only applicable if
	// leader election is enabled.
	RetryPeriod time.Duration
}

func SetSharedOpts(fs *flag.FlagSet, opts *SharedOpts) {
	fs.StringVar(&opts.ConfigPath, "config-path", "", "Path of the config file containing the configuration")
	fs.IntVar(&opts.ConcurrentReconciles, "concurrent-reconciles", defaultConcurrentReconciles, "Maximum number of concurrent reconciles")
	fs.IntVar(&opts.KubeApiBurst, "kube-api-burst", rest.DefaultBurst, "Maximum burst to throttle the calls to the API server.")
	fs.Float64Var(&opts.KubeApiQps, "kube-api-qps", float64(rest.DefaultQPS), "Maximum QPS (queries per second) allowed from the client to the API server")
	fs.StringVar(&opts.MetricsBindAddress, "metrics-bind-addr", defaultMetricsBindAddress, "The TCP address that the controller should bind to for serving prometheus metrics")
	fs.StringVar(&opts.HealthBindAddress, "health-bind-addr", defaultHealthBindAddress, "The TCP address that the controller should bind to for serving health probes")
	bindLeaderElectionFlags(fs, opts)
}

func bindLeaderElectionFlags(fs *flag.FlagSet, opts *SharedOpts) {
	fs.BoolVar(&opts.LeaderElection.Enable, "enable-leader-election", false, "Start a leader election client and gain leadership before "+
		"executing the main loop. Enable this when running replicated "+
		"components for high availability.")
	fs.StringVar(&opts.LeaderElection.Namespace, "leader-election-namespace", "garden", "Namespace in which leader election resource will be created. It should be the same namespace where DWD pods are deployed")
	fs.DurationVar(&opts.LeaderElection.LeaseDuration, "leader-elect-lease-duration", defaultLeaseDuration, "The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled.")
	fs.DurationVar(&opts.LeaderElection.RenewDeadline, "leader-elect-renew-deadline", defaultRenewDeadline, "The interval between attempts by the acting master to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled.")
	fs.DurationVar(&opts.LeaderElection.RetryPeriod, "leader-elect-retry-period", defaultRetryPeriod, "The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled.")
}
