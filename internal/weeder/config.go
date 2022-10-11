package weeder

import (
	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/gardener/dependency-watchdog/internal/util"
	multierr "github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const (
	DefaultWatchDuration = 5 * time.Minute
)

func LoadConfig(filename string) (*wapi.Config, error) {
	config, err := util.ReadAndUnmarshall[wapi.Config](filename)
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

func validate(c *wapi.Config) error {
	v := new(util.Validator)
	// Check the mandatory config parameters for which a default will not be set
	v.MustNotBeEmpty("serviceAndDependantSelectors", c.ServicesAndDependantSelectors)
	for _, ds := range c.ServicesAndDependantSelectors {
		v.MustNotBeEmpty("podSelectors", ds.PodSelectors)
		for _, selector := range ds.PodSelectors {
			_, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				v.Error = multierr.Append(v.Error, err)
				continue
			}
		}
	}
	return v.Error
}

func fillDefaultValues(c *wapi.Config) {
	if c.WatchDuration == nil {
		c.WatchDuration = &metav1.Duration{
			Duration: DefaultWatchDuration,
		}
	}
}
