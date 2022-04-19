package prober_test

import (
	"errors"
	"github.com/gardener/dependency-watchdog/internal/prober"
	multierr "github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
	"path/filepath"
)

var _ = Describe("Config", Ordered, Label("config"), func() {
	var (
		testdataPath string
	)

	BeforeAll(func() {
		testdataPath = "testdata"
		validateIfFileExists(testdataPath)
	})

	Context("valid config should pass all validations", func() {
		It("load valid config", func() {
			configPath := filepath.Join(testdataPath, "valid_config.yaml")
			validateIfFileExists(configPath)
			config, err := prober.LoadConfig(configPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).ToNot(BeNil())
			Expect(len(config.DependentResourceInfos)).To(Equal(3))
		})
	})

	Context("config missing mandatory values", func() {
		It("load invalid config", func() {
			configPath := filepath.Join(testdataPath, "config_missing_mandatory_values.yaml")
			validateIfFileExists(configPath)
			config, err := prober.LoadConfig(configPath)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
			if merr, ok := err.(*multierr.Error); ok {
				Expect(len(merr.Errors)).To(Equal(6))
			}
		})
	})

	Context("fill missing values", func() {
		It("check if default values are set for all optional missing values", func() {
			configPath := filepath.Join(testdataPath, "config_missing_optional_values.yaml")
			validateIfFileExists(configPath)
			config, err := prober.LoadConfig(configPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).ToNot(BeNil())
			Expect(config.InitialDelay.Milliseconds()).To(Equal(prober.DefaultInitialDelay.Milliseconds()))
			Expect(config.ProbeInterval.Milliseconds()).To(Equal(prober.DefaultProbeInterval.Milliseconds()))
			Expect(*config.SuccessThreshold).To(Equal(prober.DefaultSuccessThreshold))
			Expect(*config.FailureThreshold).To(Equal(prober.DefaultFailureThreshold))
			Expect(config.BackoffDuration.Milliseconds()).To(Equal(prober.DefaultBackoffDuration.Milliseconds()))
			Expect(*config.BackoffJitterFactor).To(Equal(prober.DefaultBackoffJitterFactor))
			for _, resInfo := range config.DependentResourceInfos {
				Expect(resInfo.ScaleUpInfo.InitialDelay.Milliseconds()).To(Equal(prober.DefaultInitialDelay.Milliseconds()))
				Expect(*resInfo.ScaleUpInfo.Replicas).To(Equal(prober.DefaultScaleUpReplicas))
				Expect(resInfo.ScaleUpInfo.Timeout.Milliseconds()).To(Equal(prober.DefaultScaleUpdateTimeout.Milliseconds()))
				Expect(resInfo.ScaleDownInfo.InitialDelay.Milliseconds()).To(Equal(prober.DefaultInitialDelay.Milliseconds()))
				Expect(*resInfo.ScaleDownInfo.Replicas).To(Equal(prober.DefaultScaleDownReplicas))
				Expect(resInfo.ScaleDownInfo.Timeout.Milliseconds()).To(Equal(prober.DefaultScaleUpdateTimeout.Milliseconds()))
			}
		})
	})

})

func validateIfFileExists(file string) {
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		GinkgoWriter.Printf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	Expect(err).ToNot(HaveOccurred())
}
