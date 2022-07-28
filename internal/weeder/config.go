package weeder

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/gardener/dependency-watchdog/internal/util"
	multierr "github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const (
	DefaultWatchDuration = 2 * time.Minute
)

func LoadConfig(filename string) (*wapi.Config, error) {
	config, err := readAndUnmarshall(filename)
	if err != nil {
		return nil, err
	}
	fillDefaultValues(config)
	err = validate(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func readAndUnmarshall(filename string) (*wapi.Config, error) {
	//TODO: Implement me. Try and generify it - it might not work as unmarshalling typically happens during runtime
	/*
		func readAndUnmarshall[T Any](filename string) (*T, error) {
			t := new(T)
			return yaml.Unmarshall(configBytes, t)
		}
	*/
}

func validate(c *wapi.Config) error {
	v := new(util.Validator)
	// Check the mandatory config parameters for which a default will not be set
	v.MustNotBeEmpty("serviceAndDependantSelectors", c.ServicesAndDependantSelectors)
	for _, dependants := range c.ServicesAndDependantSelectors {
		v.MustNotBeEmpty("podSelector", dependants.PodSelectors)
		for _, selector := range dependants.PodSelectors {
			_, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				v.Error = multierr.Append(v.Error, err)
				continue
			}
		}
	}
}

func fillDefaultValues(c *wapi.Config) {
	if c.WatchDuration == nil {
		c.WatchDuration = new(time.Duration)
		*c.WatchDuration = DefaultWatchDuration
	}
}
