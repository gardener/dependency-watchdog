package restarter

import (
	"io/ioutil"

	"github.com/ghodss/yaml"
)

func LoadServiceDependants(file string) (*ServiceDependants, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return decodeConfigFile(data)
}

func decodeConfigFile(data []byte) (*ServiceDependants, error) {
	dependants := new(ServiceDependants)
	err := yaml.Unmarshal(data, dependants)
	if err != nil {
		return nil, err
	}
	return dependants, nil
}
