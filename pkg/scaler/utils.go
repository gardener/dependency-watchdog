// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scaler

import (
	"io/ioutil"
	"strings"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/ghodss/yaml"
	"k8s.io/klog"
)

// LoadProbeDependantsListFile creates the ProbeDependantsList from a config-file.
func LoadProbeDependantsListFile(file string) (*ProbeDependantsList, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return DecodeConfigFile(data)
}

// DecodeConfigFile decodes the byte stream to ServiceDependants objects.
func DecodeConfigFile(data []byte) (*ProbeDependantsList, error) {
	dependants := new(ProbeDependantsList)
	err := yaml.Unmarshal(data, dependants)
	if err != nil {
		return nil, err
	}
	return dependants, nil
}

// EncodeConfigFile encodes the ProbeDependantsList objects into a string.
func EncodeConfigFile(dependants *ProbeDependantsList) (string, error) {
	data, err := yaml.Marshal(dependants)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}

	const prefix = "rate: "
	return strings.HasPrefix(err.Error(), prefix)
}

func shootWokeUp(old, new *gardenerv1alpha1.Cluster) bool {
	decoder, err := extensionscontroller.NewGardenDecoder()
	if err != nil {
		klog.V(4).Infof("Error getting gardener decoder for cluster %v. Err: %v", new.Name, err)
		return false
	}

	oldShoot, err := extensionscontroller.ShootFromCluster(decoder, old)
	if err != nil {
		klog.V(4).Infof("Error getting old shoot version from cluster: %v. Err: %v", old.Name, err)
		return false
	}
	newShoot, err := extensionscontroller.ShootFromCluster(decoder, new)
	if err != nil {
		klog.V(4).Infof("Error getting new shoot version from cluster: %v. Err: %v", new.Name, err)
		return false
	}

	if oldShoot.Status.IsHibernated == true && newShoot.Status.IsHibernated == false {
		return true
	}

	klog.V(4).Infof("Cluster: %v, Old shoot hibernation state: %v. New shoot hibernation state: %v", new.Name, oldShoot.Status.IsHibernated, newShoot.Status.IsHibernated)
	return false
}
