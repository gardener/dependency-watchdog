// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var (
	yes = true
	no  = false
)

var _ = DescribeTable("doCheckShootHibernationStateChanged", func(oldSpec *gardencorev1beta1.Hibernation, oldStatus bool, newSpec *gardencorev1beta1.Hibernation, newStatus, expectChanged bool) {
	var (
		old = &gardencorev1beta1.Shoot{
			Spec: gardencorev1beta1.ShootSpec{
				Hibernation: oldSpec,
			},
			Status: gardencorev1beta1.ShootStatus{
				IsHibernated: oldStatus,
			},
		}
		new = &gardencorev1beta1.Shoot{
			Spec: gardencorev1beta1.ShootSpec{
				Hibernation: newSpec,
			},
			Status: gardencorev1beta1.ShootStatus{
				IsHibernated: newStatus,
			},
		}
	)
	Expect(doCheckShootHibernationStateChanged(old, new)).To(Equal(expectChanged))
},
	Entry("disabled, false, disabled, false", &gardencorev1beta1.Hibernation{Enabled: &no}, false, &gardencorev1beta1.Hibernation{Enabled: &no}, false, false),
	Entry("disabled, false, enabled, false", &gardencorev1beta1.Hibernation{Enabled: &no}, false, &gardencorev1beta1.Hibernation{Enabled: &yes}, false, true),
	Entry("disabled, true, disabled, true", &gardencorev1beta1.Hibernation{Enabled: &no}, true, &gardencorev1beta1.Hibernation{Enabled: &no}, true, false),
	Entry("disabled, true, enabled, true", &gardencorev1beta1.Hibernation{Enabled: &no}, true, &gardencorev1beta1.Hibernation{Enabled: &yes}, true, true),
	Entry("enabled, false, disabled, false", &gardencorev1beta1.Hibernation{Enabled: &yes}, false, &gardencorev1beta1.Hibernation{Enabled: &no}, false, true),
	Entry("enabled, false, enabled, false", &gardencorev1beta1.Hibernation{Enabled: &yes}, false, &gardencorev1beta1.Hibernation{Enabled: &yes}, false, false),
	Entry("enabled, true, disabled, true", &gardencorev1beta1.Hibernation{Enabled: &yes}, true, &gardencorev1beta1.Hibernation{Enabled: &no}, true, true),
	Entry("enabled, true, enabled, true", &gardencorev1beta1.Hibernation{Enabled: &yes}, true, &gardencorev1beta1.Hibernation{Enabled: &yes}, true, false),
	Entry("nil, false, nil, false", nil, false, nil, false, false),
	Entry("nil, false, nil, true", nil, false, nil, true, true),
	Entry("nil, false, non-nil, false", nil, false, &gardencorev1beta1.Hibernation{}, false, false),
	Entry("nil, true, nil, false", nil, true, nil, false, true),
	Entry("nil, true, nil, true", nil, true, nil, true, false),
	Entry("nil, true, non-nil, true", nil, true, &gardencorev1beta1.Hibernation{}, true, false),
	Entry("nil-enabled, false, non-nil-enabled, false", &gardencorev1beta1.Hibernation{}, false, &gardencorev1beta1.Hibernation{Enabled: &yes}, false, true),
	Entry("nil-enabled, true, non-nil-enabled, true", &gardencorev1beta1.Hibernation{}, true, &gardencorev1beta1.Hibernation{Enabled: &yes}, true, true),
	Entry("non-nil, false, nil, false", &gardencorev1beta1.Hibernation{}, false, nil, false, false),
	Entry("non-nil, true, nil, true", &gardencorev1beta1.Hibernation{}, true, nil, true, false),
	Entry("non-nil-enabled, false, nil-enabled, false", &gardencorev1beta1.Hibernation{Enabled: &yes}, false, &gardencorev1beta1.Hibernation{}, false, true),
	Entry("non-nil-enabled, true, nil-enabled, true", &gardencorev1beta1.Hibernation{Enabled: &yes}, true, &gardencorev1beta1.Hibernation{}, true, true),
)
