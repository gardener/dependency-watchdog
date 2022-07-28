package util

import (
	"fmt"
	"reflect"
	"strings"

	multierr "github.com/hashicorp/go-multierror"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Validator struct {
	Error error
}

func (v *Validator) MustNotBeEmpty(key string, value interface{}) bool {
	if value == nil {
		v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be nil or empty", key))
		return false
	}
	cv := reflect.ValueOf(value)
	switch cv.Kind() {
	case reflect.String:
		if strings.TrimSpace(cv.String()) == "" {
			v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be empty", key))
			return false
		}
	case reflect.Slice:
		if cv.Len() == 0 {
			v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be empty", key))
			return false
		}
	case reflect.Map:
		if cv.Len() == 0 {
			v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be empty", key))
			return false
		}
	}

	return true
}

func (v *Validator) MustNotBeNil(key string, value interface{}) bool {
	if value == nil || reflect.ValueOf(value).IsNil() {
		v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be nil", key))
		return false
	}
	return true
}

func (v *Validator) ResourceRefMustBeValid(resourceRef *autoscalingv1.CrossVersionObjectReference) bool {
	_, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		v.Error = multierr.Append(v.Error, err)
		return false
	}
	return true
}
