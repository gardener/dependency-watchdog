package weeder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const CrashLoopBackOff = "CrashLoopBackOff"

// Config provides typed access weeder configuration
type Config struct {
	// WatchDuration Duration for which all dependent pods for a service under surveillance will be watched after the service has recovered.
	// If the dependent pods have not transitioned to CrashLoopBackOff in this duration then it is assumed that they will not enter that state.
	WatchDuration *time.Duration `yaml:"watchDuration,omitempty"`
	// ServicesAndDependantSelectors is a map whose key is the service name and the value is a DependantSelectors
	ServicesAndDependantSelectors map[string]DependantSelectors `yaml:"servicesAndDependantSelectors"`
}

// DependantSelectors encapsulates LabelSelector's used to identify dependants for a service
type DependantSelectors struct {
	// PodSelectors is a slice of LabelSelector's used to identify dependant pods
	PodSelectors []*metav1.LabelSelector `yaml:"podSelectors"`
}
