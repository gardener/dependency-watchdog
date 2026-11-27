// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"flag"
	"fmt"

	"github.com/gardener/dependency-watchdog/controllers/adjuster"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	// AdjusterCmd stores info about using the adjuster command
	AdjusterCmd = &Command{
		Name:      "adjuster",
		UsageLine: "",
		ShortDesc: "Checks Machine metrics/status and adjusts machine-creation-timeout on MachineDeployment's",
		LongDesc: `For the seed cluster, it will start a watch on Machine objects, check to see if they have failed to join
the shoot cluster within twice the machine-creation-timeout. If not, it will adjust the effective machine-creation-timeout
on the corresponding MachineDeployment with the same instance type and zone combination. This is adjusted upwards until
2h is reached. If all Machines belonging to an instance type & zone combo have successfully joined the cluster in a window that is 
twice the machine creation timeout, it will adjust the effective machine-creation-timeout downwards to the maximum Machine join time.

Flags:
	--config-file
		Path of the configuration file containing adjuster configuration
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
		AddFlags: addAdjusterFlags,
		Run:      startAdjusterController,
	}
	adjusterOpts = adjusterOptions{}
)

type adjusterOptions struct {
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

func addAdjusterFlags(fs *flag.FlagSet) {
	SetSharedOpts(fs, &adjusterOpts.SharedOpts)
}

func startAdjusterController(log logr.Logger) (manager.Manager, error) {
	adjusterLogger := log.WithName("adjuster-controller")
	adjusterConfig, err := adjuster.LoadConfig(adjusterOpts.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse adjuster config file %s : %w", adjusterOpts.ConfigFile, err)
	}
	log.Info("Adjuster starting with config", "adjusterConfig", adjusterConfig)

	restConf := ctrl.GetConfigOrDie()
	restConf.QPS = float32(adjusterOpts.KubeApiQps)
	restConf.Burst = adjusterOpts.KubeApiBurst

	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    server.Options{BindAddress: adjusterOpts.MetricsBindAddress},
		HealthProbeBindAddress:     adjusterOpts.HealthBindAddress,
		LeaderElection:             adjusterOpts.LeaderElection.Enable,
		LeaseDuration:              &adjusterOpts.LeaderElection.LeaseDuration,
		RenewDeadline:              &adjusterOpts.LeaderElection.RenewDeadline,
		RetryPeriod:                &adjusterOpts.LeaderElection.RetryPeriod,
		LeaderElectionNamespace:    adjusterOpts.LeaderElection.Namespace,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           adjusterLeaderElectionID,
		Logger:                     adjusterLogger,
		PprofBindAddress:           adjusterOpts.PprofBindAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start the adjuster controller manager %w", err)
	}

	adjusterController := adjuster.NewController(mgr.GetScheme(), mgr.GetClient(), adjusterConfig, adjusterOpts.ConcurrentReconciles)
	if err := adjusterController.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("failed to register adjuster controller with the controller manager %w", err)
	}

	return mgr, nil
}
