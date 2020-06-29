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
	"os"

	"github.com/gardener/dependency-watchdog/pkg/scaler"
	gardenerclientset "github.com/gardener/gardener/pkg/client/extensions/clientset/versioned"
	gardenerinformer "github.com/gardener/gardener/pkg/client/extensions/informers/externalversions"
	"github.com/spf13/cobra"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
)

// probeCmd represents the probe command
var probeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Probe kubernetes apiservers for health and scale up/down dependant controllers.",
	Long: `Probe kubernetes apiservers for health and scale down the dependant controllers 
	if the apiserver is reachable internally and not reachable externally. It scales the
	dependent controllers back up when the apiserver becomes reachable externally.`,
	Run: runProbe,
}

func init() {
	rootCmd.AddCommand(probeCmd)
}

func runProbe(cmd *cobra.Command, args []string) {
	klog.V(5).Info("Running probe command")
	klog.V(2).Infoln("Running probe command with the following parameters:")
	klog.V(2).Infoln("config-file: ", configFile)
	klog.V(2).Infoln("kubeconfig: ", kubeconfig)
	klog.V(2).Infoln("master: ", deployedNamespace)
	klog.V(2).Infoln("deployed-namespace: ", masterURL)
	klog.V(2).Infoln("concurrent-syncs: ", concurrentSyncs)
	klog.V(2).Infoln("qps: ", qps)
	klog.V(2).Infoln("burst: ", burst)
	klog.V(2).Infoln("port: ", port)

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := setupSignalHandler()
	deps, err := scaler.LoadProbeDependantsListFile(configFile)
	if err != nil {
		klog.Fatalf("Error parsing config file: %s", err.Error())
	}

	configContent, err := scaler.EncodeConfigFile(deps)
	klog.V(2).Infof("Probe configuration: \n %s", configContent)

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error parsing kubeconfig file: %s", err.Error())
	}

	config.QPS = qps
	config.Burst = burst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating k8s clientset: %s", err.Error())
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(clientset.Discovery()))
	var opts []informers.SharedInformerOption
	if deps.Namespace != "" {
		opts = append(opts, informers.WithNamespace(deps.Namespace))
	}
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		defaultSyncDuration,
		opts...)

	gardenerClientSet, err := gardenerclientset.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating k8s clientset: %s", err.Error())
	}

	gardenerInformerFactory := gardenerinformer.NewSharedInformerFactory(
		gardenerClientSet,
		defaultSyncDuration,
	)

	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(clientset.Discovery()) // DiscoveryScaleKindResolver does the caching
	scaleGetter := scale.New(clientset.RESTClient(), mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	controller := scaler.NewController(clientset, mapper, scaleGetter, factory, gardenerInformerFactory, deps, stopCh)
	leaderElectionClient := kubernetes.NewForConfigOrDie(rest.AddUserAgent(config, "dependency-watchdog-election"))
	recorder := createRecorder(leaderElectionClient)
	run := func(ctx context.Context) {
		go serveMetrics()
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
		"dependency-watchdog-probe",
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
