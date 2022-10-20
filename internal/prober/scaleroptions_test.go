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

package prober

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

var (
	timeout  = 10 * time.Millisecond
	interval = 2 * time.Millisecond
)

func TestWithDependentResourceCheckTimeout(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := scalerOptions{}
	fn := withDependentResourceCheckTimeout(timeout)
	fn(&scalerOptions)
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(timeout))
}

func TestWithDependentResourceCheckInterval(t *testing.T) {
	g := NewWithT(t)
	scaleroptions := scalerOptions{}
	fn := withDependentResourceCheckInterval(interval)
	fn(&scaleroptions)
	g.Expect(*scaleroptions.dependentResourceCheckInterval).To(Equal(interval))
}

func TestBuildScalerOptions(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := buildScalerOptions(withDependentResourceCheckTimeout(timeout), withDependentResourceCheckInterval(interval))
	g.Expect(*scalerOptions.dependentResourceCheckInterval).To(Equal(interval))
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(timeout))
}

func TestBuildScalerOptionsShouldFillDefaultValues(t *testing.T) {
	g := NewWithT(t)
	scalerOptions := buildScalerOptions()
	g.Expect(*scalerOptions.dependentResourceCheckInterval).To(Equal(defaultDependentResourceCheckInterval))
	g.Expect(*scalerOptions.dependentResourceCheckTimeout).To(Equal(defaultDependentResourceCheckTimeout))
}
