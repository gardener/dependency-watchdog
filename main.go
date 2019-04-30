// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gardener/dep-controller/pkg/restarter"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"

	"k8s.io/client-go/informers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

const (
	defaultWatchDuration = "2m"
)

var (
	masterURL        string
	configFile       string
	kubeconfig       string
	strWatchDuration string

	defaultSyncDuration  = 30 * time.Second
	onlyOneSignalHandler = make(chan struct{})
	shutdownSignals      = []os.Signal{os.Interrupt, syscall.SIGTERM}
	labelSelector        = labels.Set(map[string]string{"app": "etcd-statefulset", "role": "main"}).AsSelector()
)

func init() {
	pflag.StringVar(&configFile, "config-file", "config.yaml", "path to the config file that has the service depenancies")
	pflag.StringVar(&kubeconfig, "kubeconfig", "kubeconfig.yaml", "path to the kube config file")
	pflag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	pflag.StringVar(&strWatchDuration, "watch-duration", defaultWatchDuration, "The duration to watch dependencies after the service is ready.")
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

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
	klog.Infof("Dependencies: %+v", deps)
	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error parsing kubeconfig file: %s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating k8s clientset: %s", err.Error())
	}

	klog.Info(labelSelector.String())
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		defaultSyncDuration,
		informers.WithNamespace(deps.Namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labelSelector.String()
		}))

	controller := restarter.NewController(clientset, factory, deps, watchDuration, stopCh)

	klog.Info("Starting informer factory.")
	factory.Start(stopCh)
	klog.Info("Starting endpoint controller.")
	if err = controller.Run(1, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}

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
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
