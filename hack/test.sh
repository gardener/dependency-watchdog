#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "> test"

ENVTEST_K8S_VERSION="1.26"
export KUBEBUILDER_ASSETS="$(setup-envtest --os $(go env GOOS) --arch $(go env GOARCH) use $ENVTEST_K8S_VERSION -p path)"
echo "Running tests using KUBEBUILDER_ASSETS=$KUBEBUILDER_ASSETS"
export KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT=true
# Tests using envtest needs to be serialized as there are issues in starting more than one envtest concurrently.
# see https://github.com/kubernetes-sigs/controller-runtime/issues/1363 which remains unresolved.
go test -v ./controllers/cluster
go test -v ./controllers/endpoint
go test -v ./internal/...


