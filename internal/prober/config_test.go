package prober

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	multierr "github.com/hashicorp/go-multierror"
	. "github.com/onsi/gomega"
)

const testdataPath = "testdata"

func TestCheckIfDefaultValuesAreSetForAllOptionalMissingValues(t *testing.T) {
	g := NewWithT(t)
	validateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "config_missing_optional_values.yaml")
	validateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)

	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give any error for a valid config file")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should not return nil for a valid config file")
	g.Expect(config.InitialDelay.Milliseconds()).To(Equal(DefaultInitialDelay.Milliseconds()), "LoadConfig should set initial delay to DefaultInitialDelay if not set in the config file")
	g.Expect(config.ProbeInterval.Milliseconds()).To(Equal(DefaultProbeInterval.Milliseconds()), "LoadConfig should set probe delay to DefaultProbeInterval if not set in the config file")
	g.Expect(*config.SuccessThreshold).To(Equal(DefaultSuccessThreshold), "LoadConfig should set success threshold to DefaultSuccessThreshold if not set in the config file")
	g.Expect(*config.FailureThreshold).To(Equal(DefaultFailureThreshold), "LoadConfig should set failure threshold to DefaultFailureThreshold if not set in the config file")
	g.Expect(config.BackoffDuration.Milliseconds()).To(Equal(DefaultBackoffDuration.Milliseconds()), "LoadConfig should set backOff duration to DefaultBackoffDuration if not set in the config file")
	g.Expect(*config.BackoffJitterFactor).To(Equal(DefaultBackoffJitterFactor), "LoadConfig should set jitter factor to DefaultJitterFactor if not set in the config file")
	for _, resInfo := range config.DependentResourceInfos {
		g.Expect(resInfo.ScaleUpInfo.InitialDelay.Milliseconds()).To(Equal(DefaultInitialDelay.Milliseconds()), fmt.Sprintf("LoadConfig should set scale up initial delay for %v to DefaultInitialDelay if not set in the config file", resInfo.Ref.Name))
		g.Expect(*resInfo.ScaleUpInfo.Replicas).To(Equal(DefaultScaleUpReplicas), fmt.Sprintf("LoadConfig should set scale up replicas for %v to DefaultScaleUpReplicas if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleUpInfo.Timeout.Milliseconds()).To(Equal(DefaultScaleUpdateTimeout.Milliseconds()), fmt.Sprintf("LoadConfig should set scale up timeout for %v to DefaultScaleUpTimeout if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleDownInfo.InitialDelay.Milliseconds()).To(Equal(DefaultInitialDelay.Milliseconds()), fmt.Sprintf("LoadConfig should set scale down initial delay for %v to DefaultInitialDelay if not set in the config file", resInfo.Ref.Name))
		g.Expect(*resInfo.ScaleDownInfo.Replicas).To(Equal(DefaultScaleDownReplicas), fmt.Sprintf("LoadConfig should set scale down replicas for %v to DefaultScaleDownReplicas if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleDownInfo.Timeout.Milliseconds()).To(Equal(DefaultScaleUpdateTimeout.Milliseconds()), fmt.Sprintf("LoadConfig should set scale down timeout for %v to DefaultScaleDownTimeout if not set in the config file", resInfo.Ref.Name))
	}

	t.Log("All missing values are set")
}

func TestMissingConfigValuesShouldReturnErrorAndNilConfig(t *testing.T) {
	g := NewWithT(t)
	validateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "config_missing_mandatory_values.yaml")
	validateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)

	g.Expect(err).To(HaveOccurred(), "LoadConfig should return error for a config with missing mandatory values")
	g.Expect(config).To(BeNil(), "LoadConfig should return a nil config for a file with missing mandatory values")
	if merr, ok := err.(*multierr.Error); ok {
		g.Expect(len(merr.Errors)).To(Equal(6), "LoadConfig did not return all the errors for a faulty config")
	}

	t.Log("All the missing mandatory values are identified")
}

func TestValidConfigShouldPassAllValidations(t *testing.T) {
	g := NewWithT(t)
	validateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "valid_config.yaml")
	validateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)
	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give error for a valid config")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should got nil config for a valid file")
	g.Expect(len(config.DependentResourceInfos)).To(Equal(3), "LoadConfig did not load all the dependent resources")

	t.Log("Valid config is loaded correctly")
}

func validateIfFileExists(file string, t *testing.T) {
	g := NewWithT(t)
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(HaveOccurred(), "File at path %v should exist")
}
