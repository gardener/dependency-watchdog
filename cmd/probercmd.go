// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"flag"
	"fmt"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/gardener/dependency-watchdog/controllers/cluster"
	"github.com/gardener/dependency-watchdog/internal/prober"
	"github.com/gardener/dependency-watchdog/internal/util"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	proberLeaderElectionID = "dwd-prober-leader-election"
	weederLeaderElectionID = "dwd-weeder-leader-election"
)

var (
	// ProberCmd stores info about using the prober command
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
	--kubeconfig
		Path to the kubeconfig file. If not specified, then it will default to the service account token to connect to the kube-api-server
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
		AddFlags: addProbeFlags,
		Run:      startClusterControllerMgr,
	}
	proberOpts = proberOptions{}
	scheme     = runtime.NewScheme()
)

type proberOptions struct {
	SharedOpts
}

func init() {
	localSchemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		extensionsv1alpha1.AddToScheme,
		machinev1alpha1.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(scheme))
}

func addProbeFlags(fs *flag.FlagSet) {
	SetSharedOpts(fs, &proberOpts.SharedOpts)
}

func startClusterControllerMgr(logger logr.Logger) (manager.Manager, error) {
	proberLogger := logger.WithName("cluster-controller")
	proberConfig, err := prober.LoadConfig(proberOpts.ConfigFile, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to parse prober config file %s : %w", proberOpts.ConfigFile, err)
	}

	restConf := ctrl.GetConfigOrDie()
	restConf.QPS = float32(proberOpts.KubeApiQps)
	restConf.Burst = proberOpts.KubeApiBurst

	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    server.Options{BindAddress: proberOpts.SharedOpts.MetricsBindAddress},
		HealthProbeBindAddress:     proberOpts.SharedOpts.HealthBindAddress,
		LeaderElection:             proberOpts.SharedOpts.LeaderElection.Enable,
		LeaseDuration:              &proberOpts.SharedOpts.LeaderElection.LeaseDuration,
		RenewDeadline:              &proberOpts.SharedOpts.LeaderElection.RenewDeadline,
		RetryPeriod:                &proberOpts.SharedOpts.LeaderElection.RetryPeriod,
		LeaderElectionNamespace:    proberOpts.SharedOpts.LeaderElection.Namespace,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           proberLeaderElectionID,
		Logger:                     proberLogger,
		PprofBindAddress:           proberOpts.SharedOpts.PprofBindAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start the prober controller manager %w", err)
	}

	scalesGetter, err := util.CreateScalesGetter(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, fmt.Errorf("failed to create clientSet for scalesGetter %w", err)
	}

	if err := (&cluster.Reconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		ScaleGetter:             scalesGetter,
		ProberMgr:               prober.NewManager(),
		DefaultProbeConfig:      proberConfig,
		MaxConcurrentReconciles: proberOpts.ConcurrentReconciles,
	}).SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to register cluster reconciler with the prober controller manager %w", err)
	}
	return mgr, nil
}
