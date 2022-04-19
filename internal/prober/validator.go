package prober

import (
	"fmt"
	"reflect"
	"strings"

	multierr "github.com/hashicorp/go-multierror"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type validator struct {
	error
}

func (v *validator) mustNotBeEmpty(key string, value interface{}) bool {
	if value == nil {
		v.error = multierr.Append(v.error, fmt.Errorf("%s must not be nil or empty", key))
		return false
	}
	cv := reflect.ValueOf(value)
	switch cv.Kind() {
	case reflect.String:
		if strings.TrimSpace(cv.String()) == "" {
			v.error = multierr.Append(v.error, fmt.Errorf("%s must not be empty", key))
			return false
		}
	case reflect.Slice:
		if cv.Len() == 0 {
			v.error = multierr.Append(v.error, fmt.Errorf("%s must not be empty", key))
			return false
		}
	}
	return true
}

func (v *validator) mustNotBeNil(key string, value interface{}) bool {
	if value == nil || reflect.ValueOf(value).IsNil() {
		v.error = multierr.Append(v.error, fmt.Errorf("%s must not be nil", key))
		return false
	}
	return true
}

func (v *validator) resourceRefMustBeValid(resourceRef autoscalingv1.CrossVersionObjectReference) bool {
	_, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		v.error = multierr.Append(v.error, err)
		return false
	}
	return true
}
