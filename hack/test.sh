#!/usr/bin/env bash
#
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

echo "> test"

ENVTEST_K8S_VERSION="1.26"
export KUBEBUILDER_ASSETS="$(setup-envtest --os $(go env GOOS) --arch $(go env GOARCH) use $ENVTEST_K8S_VERSION -p path)"
echo "Running tests using KUBEBUILDER_ASSETS=$KUBEBUILDER_ASSETS"
export KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT=true
# Tests using envtest needs to be serialized as there are issues in starting more than one envtest concurrently.
# see https://github.com/kubernetes-sigs/controller-runtime/issues/1363 which remains unresolved.
go test -json -cover ./controllers/cluster | gotestfmt -hide empty-packages
go test -json -cover ./controllers/endpoint | gotestfmt -hide empty-packages
go test -json -cover `go list ./internal/... | grep -v mock | grep -v test` | gotestfmt -hide empty-packages
