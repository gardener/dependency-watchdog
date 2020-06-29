module github.com/gardener/dependency-watchdog

go 1.13

require (
	github.com/gardener/gardener v1.6.5
	github.com/ghodss/yaml v1.0.0
	github.com/onsi/ginkgo v1.12.2
	github.com/onsi/gomega v1.10.1
	github.com/prometheus/client_golang v1.3.0
	github.com/spf13/cobra v0.0.6
	github.com/spf13/pflag v1.0.5
	golang.org/x/sys v0.0.0-20200523222454-059865788121 // indirect
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/component-base v0.18.2
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.6.0
)
