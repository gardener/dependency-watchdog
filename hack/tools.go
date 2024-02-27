// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
	_ "github.com/golang/mock/mockgen/model"
	_ "k8s.io/code-generator/cmd/import-boss"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
)
