// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	multierr "github.com/hashicorp/go-multierror"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Validator is a struct to store all validation errors.
type Validator struct {
	Error error
}

// MustNotBeEmpty checks whether the given value is empty. It returns false if it is empty or nil.
func (v *Validator) MustNotBeEmpty(key string, value any) bool {
	if value == nil {
		v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be nil or empty", key))
		return false
	}
	cv := reflect.ValueOf(value)
	switch cv.Kind() {
	case reflect.String:
		if strings.TrimSpace(cv.String()) == "" {
			v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must not be empty", key))
			return false
		}
	case reflect.Slice:
		if cv.Len() == 0 {
			v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must not be empty", key))
			return false
		}
	case reflect.Map:
		if cv.Len() == 0 {
			v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must not be empty", key))
			return false
		}
	default:
		v.Error = multierr.Append(v.Error, fmt.Errorf("unsupported type of value for key %s. do not know how to check if it is empty", key))
		return false
	}
	return true
}

// MustNotBeZeroDuration checks whether the given duration is zero. It returns false if it is zero.
func (v *Validator) MustNotBeZeroDuration(key string, duration metav1.Duration) bool {
	if duration.Seconds() == 0 {
		v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must not be zero", key))
		return false
	}
	return true
}

// MustBeGreater checks whether the given value is greater than specified reference value, returns false otherwise.
// value types are expected to satisfy [cmp.Ordered].
// TODO: Simplify after Go 1.27 which supports generic methods — methods that can declare their own type parameters.
func (v *Validator) MustBeGreater(key string, val, refVal any) bool {
	if val == nil || refVal == nil {
		v.Error = multierr.Append(v.Error, fmt.Errorf("value/reference value for key %s must be not be nil", key))
		return false
	}
	switch v1 := val.(type) {
	case int:
		if v2, ok := refVal.(int); ok && v1 > v2 {
			return true
		}
	case int8:
		if v2, ok := refVal.(int8); ok && v1 > v2 {
			return true
		}
	case int16:
		if v2, ok := refVal.(int16); ok && v1 > v2 {
			return true
		}
	case int32:
		if v2, ok := refVal.(int32); ok && v1 > v2 {
			return true
		}
	case int64:
		if v2, ok := refVal.(int64); ok && v1 > v2 {
			return true
		}
	case uint:
		if v2, ok := refVal.(uint); ok && v1 > v2 {
			return true
		}
	case uint8:
		if v2, ok := refVal.(uint8); ok && v1 > v2 {
			return true
		}
	case uint16:
		if v2, ok := refVal.(uint16); ok && v1 > v2 {
			return true
		}
	case uint32:
		if v2, ok := refVal.(uint32); ok && v1 > v2 {
			return true
		}
	case uint64:
		if v2, ok := refVal.(uint64); ok && v1 > v2 {
			return true
		}
	case uintptr:
		if v2, ok := refVal.(uintptr); ok && v1 > v2 {
			return true
		}
	case float32:
		if v2, ok := refVal.(float32); ok && v1 > v2 {
			return true
		}
	case float64:
		if v2, ok := refVal.(float64); ok && v1 > v2 {
			return true
		}
	case string:
		if v2, ok := refVal.(string); ok && v1 > v2 {
			return true
		}
	}
	v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must be greater than %v but is %v", key, refVal, val))
	return false
}

// MustNotBeNil checks whether the given value is nil and returns false if it is nil.
func (v *Validator) MustNotBeNil(key string, value any) bool {
	if value == nil || reflect.ValueOf(value).IsNil() {
		v.Error = multierr.Append(v.Error, fmt.Errorf("%s must not be nil", key))
		return false
	}
	return true
}

// ResourceRefMustBeValid validates the given resourceRef by parsing the apiVersion.
func (v *Validator) ResourceRefMustBeValid(resourceRef *autoscalingv1.CrossVersionObjectReference, scheme *runtime.Scheme) bool {
	gv, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		v.Error = multierr.Append(v.Error, err)
		return false
	}
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    resourceRef.Kind,
	}
	return scheme.Recognizes(gvk)
}
