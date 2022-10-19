package weeder

import (
	"fmt"
	"path/filepath"
	"testing"

	multierr "github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"

	"github.com/gardener/dependency-watchdog/internal/util"
)

const testdataPath = "testdata"

func TestCheckIfDefaultValuesAreSetForAllOptionalMissingValues(t *testing.T) {
	g := NewWithT(t)
	util.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "config_missing_optional_values.yaml")
	util.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)

	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give any error for a valid config file")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should not return nil for a valid config file")
	g.Expect(*config.WatchDuration).To(Equal(metav1.Duration{Duration: DefaultWatchDuration}), "LoadConfig should set watchDuration to DefaultWatchDuration if not set in the config file")
	t.Log("All default values are set")
}

func TestMissingMandatoryFieldsShouldReturnErrorAndNilConfig(t *testing.T) {
	table := []struct {
		fileName         string
		expectedErrCount int
	}{
		{"config_missing_mandatory_values.yaml", 1},
		{"config_missing_mandatory_values_2.yaml", 1},
	}

	for _, entry := range table {
		g := NewWithT(t)
		util.ValidateIfFileExists(testdataPath, t)
		configPath := filepath.Join(testdataPath, entry.fileName)
		util.ValidateIfFileExists(configPath, t)
		config, err := LoadConfig(configPath)

		fmt.Println(err)
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
	util.ValidateIfFileExists(testdataPath, t)

	configPath := filepath.Join(testdataPath, "valid_config.yaml")
	util.ValidateIfFileExists(configPath, t)
	config, err := LoadConfig(configPath)
	g.Expect(err).ToNot(HaveOccurred(), "LoadConfig should not give error for a valid config")
	g.Expect(config).ToNot(BeNil(), "LoadConfig should got nil config for a valid file")
	g.Expect(len(config.ServicesAndDependantSelectors)).To(Equal(2), "LoadConfig did not load all the dependent resources")

	t.Log("Valid config is loaded correctly")
}
