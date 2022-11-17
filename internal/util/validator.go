// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"fmt"
	"reflect"
	"strings"

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
func (v *Validator) MustNotBeEmpty(key string, value interface{}) bool {
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

// MustNotBeNil checks whether the given value is nil and returns false if it is nil.
func (v *Validator) MustNotBeNil(key string, value interface{}) bool {
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
