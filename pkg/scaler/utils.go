// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"io/ioutil"
	"strings"

	"github.com/gardener/dependency-watchdog/pkg/scaler/api"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/klog"
)

// LoadProbeDependantsListFile creates the ProbeDependantsList from a config-file.
func LoadProbeDependantsListFile(file string) (*api.ProbeDependantsList, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return api.Decode(data)
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}

	const prefix = "rate: "
	return strings.HasPrefix(err.Error(), prefix)
}

func shootHibernationStateChanged(old, new *gardenerv1alpha1.Cluster) bool {
	decoder := extensionscontroller.NewGardenDecoder()

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

	return doCheckShootHibernationStateChanged(oldShoot, newShoot)
}

func doCheckShootHibernationStateChanged(oldShoot, newShoot *gardencorev1beta1.Shoot) bool {
	return oldShoot.Status.IsHibernated != newShoot.Status.IsHibernated || gardencorev1beta1helper.HibernationIsEnabled(oldShoot) != gardencorev1beta1helper.HibernationIsEnabled(newShoot)
}
