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

// MustBeGreater checks whether the given value is greater than specified compare value, returns false otherwise.
// value types are expected to satisfy [cmp.Ordered].
// TODO: Simplify after Go 1.27 which supports generic methods — methods that can declare their own type parameters.
func (v *Validator) MustBeGreater(key string, val, compareVal any) bool {
	if val == nil || compareVal == nil {
		v.Error = multierr.Append(v.Error, fmt.Errorf("value & compare value for key %s must be not be nil", key))
		return false
	}
	t1 := reflect.TypeOf(val)
	t2 := reflect.TypeOf(compareVal)

	if t1 != t2 {
		v.Error = multierr.Append(v.Error, fmt.Errorf("for key %s, value of type %q differs from compare value of type %q", key, t1, t2))
		return false
	}
	v1 := reflect.ValueOf(val)
	v2 := reflect.ValueOf(compareVal)

	switch v1.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v1.Int() > v2.Int() {
			return true
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if v1.Uint() > v2.Uint() {
			return true
		}

	case reflect.Float32, reflect.Float64:
		if v1.Float() > v2.Float() {
			return true
		}
	case reflect.String:
		if v1.String() > v2.String() {
			return true
		}
	default:
		v.Error = multierr.Append(v.Error, fmt.Errorf("key %s cannot be compared due to unsupported value type %s", key, t1))
		return false
	}

	v.Error = multierr.Append(v.Error, fmt.Errorf("value for key %s must be greater than %v but is instead %v", key, compareVal, val))
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
