package weeder

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
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
	for _, dependants := range c.ServicesAndDependantSelectors {
		var allErrs []error
		for _, selector := range dependants.PodSelectors {
			_, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				allErrs = append(allErrs, err)
				continue
			}
			//TODO: move prober/validator.go to /internal/util so that it can be used here as well.
		}
	}
}

func fillDefaultValues(c *wapi.Config) {
	if c.WatchDuration == nil {
		c.WatchDuration = new(time.Duration)
		*c.WatchDuration = DefaultWatchDuration
	}
}
