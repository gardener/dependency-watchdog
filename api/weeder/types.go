package weeder

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Config provides typed access weeder configuration
type Config struct {
	// ServicesAndDependantSelectors is a map whose key is the service name and the value is a DependantSelectors
	ServicesAndDependantSelectors map[string]DependantSelectors `yaml:"servicesAndDependantSelectors"`
}

// DependantSelectors encapsulates LabelSelector's used to identify dependants for a service
type DependantSelectors struct {
	// PodSelectors is a slice of LabelSelector's used to identify dependant pods
	PodSelectors []*metav1.LabelSelector `yaml:"podSelectors"`
}
