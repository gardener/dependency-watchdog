package prober

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

var (
	timeout  = 10 * time.Millisecond
	interval = 2 * time.Millisecond
)

func TestWithDependentResourceCheckTimeout(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := scalerOptions{}
	fn := withDependentResourceCheckTimeout(timeout)
	fn(&scalerOptions)
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(timeout))
}

func TestWithDependentResourceCheckInterval(t *testing.T) {
	g := NewWithT(t)
	scaleroptions := scalerOptions{}
	fn := withDependentResourceCheckInterval(interval)
	fn(&scaleroptions)
	g.Expect(*scaleroptions.dependentResourceCheckInterval).To(Equal(interval))
}

func TestBuildScalerOptions(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := buildScalerOptions(withDependentResourceCheckTimeout(timeout), withDependentResourceCheckInterval(interval))
	g.Expect(*scalerOptions.dependentResourceCheckInterval).To(Equal(interval))
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(timeout))
}

func TestBuildScalerOptionsShouldFillDefaultValues(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := buildScalerOptions()
	g.Expect(*scalerOptions.dependentResourceCheckInterval).To(Equal(defaultDependentResourceCheckInterval))
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(defaultDependentResourceCheckTimeout))
}
