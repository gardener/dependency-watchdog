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
	defaultBackoffPeriod = 5
	defaultRetries       = 3
)

var (
	masterURL     string
	configFile    string
	kubeconfig    string
	backoffPeriod int
	retries       int

	defaultSyncDuration  = 30 * time.Second
	onlyOneSignalHandler = make(chan struct{})
	shutdownSignals      = []os.Signal{os.Interrupt, syscall.SIGTERM}
	labelSelector        = labels.Set(map[string]string{"app": "etcd-statefulset", "role": "main"}).AsSelector()
)

func init() {
	pflag.StringVar(&configFile, "config-file", "config.yaml", "path to the config file that has the service depenancies")
	pflag.StringVar(&kubeconfig, "kubeconfig", "kubeconfig.yaml", "path to the kube config file")
	pflag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	pflag.IntVar(&backoffPeriod, "backoff-interval", defaultBackoffPeriod, "The duration in between successive retries.")
	pflag.IntVar(&retries, "retries", defaultRetries, "The number of retries to during changes to endpoint")
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

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

	controller := restarter.NewController(clientset, factory, deps, retries, time.Duration(backoffPeriod)*time.Second, stopCh)

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
