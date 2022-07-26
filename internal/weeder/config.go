package weeder

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func LoadConfig (filename string) (*wapi.Config, error) {
	config, err := readAndUnmarshall(filename)
	if err != nil {
		return nil, err
	}
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
		for _, ls := dependants.PodSelectors {
			i_, err := metav1.LabelSelectorAsSelector(ls)
			// collect the errors into multi-err
			//TODO: move prober/validator.go to /internal/util so that it can be used here as well.
		}
	}
}