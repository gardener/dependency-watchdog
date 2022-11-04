// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prober

import (
	"fmt"
	"path/filepath"
	"testing"

	testutil "github.com/gardener/dependency-watchdog/internal/test"
	multierr "github.com/hashicorp/go-multierror"
	. "github.com/onsi/gomega"
)

const testdataPath = "testdata"

func TestCheckIfDefaultValuesAreSetForAllOptionalMissingValues(t *testing.T) {
	g := NewWithT(t)
	testutil.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "config_missing_optional_values.yaml")
	testutil.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)

	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give any error for a valid config file")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should not return nil for a valid config file")
	g.Expect(config.InitialDelay.Milliseconds()).To(Equal(DefaultProbeInitialDelay.Milliseconds()), "LoadConfig should set initial delay to DefaultInitialDelay if not set in the config file")
	g.Expect(config.ProbeInterval.Milliseconds()).To(Equal(DefaultProbeInterval.Milliseconds()), "LoadConfig should set probe delay to DefaultProbeInterval if not set in the config file")
	g.Expect(*config.SuccessThreshold).To(Equal(DefaultSuccessThreshold), "LoadConfig should set success threshold to DefaultSuccessThreshold if not set in the config file")
	g.Expect(*config.FailureThreshold).To(Equal(DefaultFailureThreshold), "LoadConfig should set failure threshold to DefaultFailureThreshold if not set in the config file")
	g.Expect(config.InternalProbeFailureBackoffDuration.Milliseconds()).To(Equal(DefaultInternalProbeFailureBackoffDuration.Milliseconds()), "LoadConfig should set backOff duration to DefaultInternalProbeFailureBackoffDuration if not set in the config file")
	g.Expect(*config.BackoffJitterFactor).To(Equal(DefaultBackoffJitterFactor), "LoadConfig should set jitter factor to DefaultJitterFactor if not set in the config file")
	for _, resInfo := range config.DependentResourceInfos {
		g.Expect(resInfo.ScaleUpInfo.InitialDelay.Milliseconds()).To(Equal(DefaultScaleInitialDelay.Milliseconds()), fmt.Sprintf("LoadConfig should set scale up initial delay for %v to DefaultInitialDelay if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleUpInfo.Timeout.Milliseconds()).To(Equal(DefaultScaleUpdateTimeout.Milliseconds()), fmt.Sprintf("LoadConfig should set scale up timeout for %v to DefaultScaleUpTimeout if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleDownInfo.InitialDelay.Milliseconds()).To(Equal(DefaultScaleInitialDelay.Milliseconds()), fmt.Sprintf("LoadConfig should set scale down initial delay for %v to DefaultInitialDelay if not set in the config file", resInfo.Ref.Name))
		g.Expect(resInfo.ScaleDownInfo.Timeout.Milliseconds()).To(Equal(DefaultScaleUpdateTimeout.Milliseconds()), fmt.Sprintf("LoadConfig should set scale down timeout for %v to DefaultScaleDownTimeout if not set in the config file", resInfo.Ref.Name))
	}
	t.Log("All missing values are set")
}

func TestMissingConfigValuesShouldReturnErrorAndNilConfig(t *testing.T) {
	table := []struct {
		fileName         string
		expectedErrCount int
	}{
		{"config_missing_mandatory_values.yaml", 9},
		{"config_missing_dependent_resource_infos.yaml", 3},
	}

	for _, entry := range table {
		g := NewWithT(t)
		testutil.ValidateIfFileExists(testdataPath, t)
		configPath := filepath.Join(testdataPath, entry.fileName)
		testutil.ValidateIfFileExists(configPath, t)
		config, err := LoadConfig(configPath)

		g.Expect(err).To(HaveOccurred(), "LoadConfig should return error for a config with missing mandatory values")
		g.Expect(config).To(BeNil(), "LoadConfig should return a nil config for a file with missing mandatory values")
		if merr, ok := err.(*multierr.Error); ok {
			g.Expect(len(merr.Errors)).To(Equal(entry.expectedErrCount), "LoadConfig did not return all the errors for a faulty config")
		}
	}
	t.Log("All the missing mandatory values are identified")
}

func TestConfigFileNotFound(t *testing.T) {
	g := NewWithT(t)
	config, err := LoadConfig(filepath.Join(testdataPath, "notfound.yaml"))
	g.Expect(err).To(HaveOccurred(), "LoadConfig should give error if config file is not found")
	g.Expect(config).To(BeNil(), "LoadConfig should return a nil config if config file is not found")
	g.Expect(err.Error()).To(ContainSubstring("no such file or directory"), "LoadConfig did not load all the dependent resources")
}

func TestErrorInUnMarshallingYaml(t *testing.T) {
	g := NewWithT(t)
	testutil.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "invalidsyntax.yaml")
	testutil.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)
	g.Expect(err).To(HaveOccurred(), "LoadConfig should not give error for a valid config")
	g.Expect(config).To(BeNil(), "LoadConfig should got nil config for a valid file")
	g.Expect(err.Error()).To(ContainSubstring("cannot unmarshal string into Go struct field DependentResourceInfo.dependentResourceInfos.scaleUp"), "Wrong error recieved")
}

func TestValidConfigShouldPassAllValidations(t *testing.T) {
	g := NewWithT(t)
	testutil.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "valid_config.yaml")
	testutil.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)
	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give error for a valid config")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should got nil config for a valid file")
	g.Expect(len(config.DependentResourceInfos)).To(Equal(3), "LoadConfig did not load all the dependent resources")

	t.Log("Valid config is loaded correctly")
}
