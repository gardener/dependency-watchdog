/*
Copyright © 2019 SAP SE or an SAP affiliate company. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gardener/dependency-watchdog/pkg/restarter"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
)

const (
	defaultWatchDuration   = "2m"
	defaultConcurrentSyncs = 1
)

var (
	masterURL                   string
	configFile                  string
	kubeconfig                  string
	deployedNamespace           string
	strWatchDuration            string
	dependencyWatchdogAgentName = "dependency-watchdog"
	defaultSyncDuration         = 30 * time.Second
	concurrentSyncs             = defaultConcurrentSyncs

	onlyOneSignalHandler = make(chan struct{})
	shutdownSignals      = []os.Signal{os.Interrupt, syscall.SIGTERM}
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dependency-watchdog",
	Short: "A watchdog component to watch and take action on Kubernetes resources.",
	Long: `A watchdog compoment to watch and take action on Kubernetes resources.
By default it watches on Kubernetes Endpoints and when they transition from unavailable
to available state it tries to wake up dependent pods from CrashloopBackoff.

Alernatively, it can also poll a pair of internal and external HTTP endpoints for the same service
and if the service is accessible internally but not externally, then scale down some dependant
components`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: runRoot,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.PersistentFlags().StringVar(&configFile, "config-file", "config.yaml", "path to the config file that has the service depenancies")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to the kube config file")
	rootCmd.PersistentFlags().StringVar(&deployedNamespace, "deployed-namespace", "default", "namespace into which the dependency-watchdog is deployed")
	rootCmd.PersistentFlags().StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	rootCmd.PersistentFlags().IntVar(&concurrentSyncs, "concurrent-syncs", defaultConcurrentSyncs, "The number of workers performing reconcilation concurrently.")
	rootCmd.Flags().StringVar(&strWatchDuration, "watch-duration", defaultWatchDuration, "The duration to watch dependencies after the service is ready.")

	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
}

func runRoot(cmd *cobra.Command, args []string) {
	klog.V(5).Info("Running root command")

	watchDuration, err := time.ParseDuration(strWatchDuration)
	if err != nil {
		klog.Fatalf("Unhandled watch duration %s: %s", strWatchDuration, err)
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := setupSignalHandler()
	deps, err := restarter.LoadServiceDependants(configFile)
	if err != nil {
		klog.Fatalf("Error parsing config file: %s", err.Error())
	}

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error parsing kubeconfig file: %s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating k8s clientset: %s", err.Error())
	}

	var opts []informers.SharedInformerOption
	if deps.Namespace != "" {
		opts = append(opts, informers.WithNamespace(deps.Namespace))
	}
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		defaultSyncDuration,
		opts...)
	controller := restarter.NewController(clientset, factory, deps, watchDuration, stopCh)
	leaderElectionClient := kubernetes.NewForConfigOrDie(rest.AddUserAgent(config, "dependency-watchdog-election"))
	recorder := createRecorder(leaderElectionClient)
	run := func(ctx context.Context) {

		klog.Info("Starting endpoint controller.")
		if err = controller.Run(concurrentSyncs); err != nil {
			klog.Fatalf("Error running controller: %s", err.Error())
		}
		panic("unreachable")
	}
	if !*controller.LeaderElection.LeaderElect {
		run(nil)
	}

	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("error fetching hostname: %v", err)
	}

	rl, err := resourcelock.New(controller.LeaderElection.ResourceLock,
		deployedNamespace,
		"dependency-watchdog",
		leaderElectionClient.CoreV1(),
		leaderElectionClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: recorder,
		})
	if err != nil {
		klog.Fatalf("error creating lock: %v", err)
	}

	ctx := context.TODO()
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: controller.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: controller.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   controller.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				klog.Fatalf("leaderelection lost")
			},
		},
	})
	panic("unreachable")
}

func createRecorder(kubeClient *kubernetes.Clientset) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})
	return eventBroadcaster.NewRecorder(kubescheme.Scheme, v1.EventSource{Component: dependencyWatchdogAgentName})
}

// setupSignalHandler registered for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
func setupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		klog.Info("Received signal %s. Stopping the controller.", <-c)
		close(stop)
		klog.Info("Received signal %s. Exiting directly.", <-c)
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
