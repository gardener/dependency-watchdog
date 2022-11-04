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

package weeder

import (
	"path/filepath"
	"testing"

	testutil "github.com/gardener/dependency-watchdog/internal/test"
	multierr "github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	g.Expect(*config.WatchDuration).To(Equal(metav1.Duration{Duration: defaultWatchDuration}), "LoadConfig should set watchDuration to defaultWatchDuration if not set in the config file")
	t.Log("All default values are set")
}

func TestMissingMandatoryFieldsShouldReturnErrorAndNilConfig(t *testing.T) {
	table := []struct {
		fileName         string
		expectedErrCount int
	}{
		{"config_missing_mandatory_values.yaml", 1},
		{"config_missing_pod_selectors.yaml", 1},
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

func TestValidConfigShouldPassAllValidations(t *testing.T) {
	g := NewWithT(t)
	testutil.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "valid_config.yaml")
	testutil.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)
	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give error for a valid config")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should got nil config for a valid file")
	g.Expect(len(config.ServicesAndDependantSelectors)).To(Equal(2), "LoadConfig did not load all the dependent resources")

	t.Log("Valid config is loaded correctly")
}
