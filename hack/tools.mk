# Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

# This make file is supposed to be included in the top-level make file.

TOOLS_DIR := hack/tools
TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin
GOLANGCI_LINT              := $(TOOLS_BIN_DIR)/golangci-lint
GO_VULN_CHECK              := $(TOOLS_BIN_DIR)/govulncheck
GOIMPORTS                  := $(TOOLS_BIN_DIR)/goimports

#default tool versions
GOLANGCI_LINT_VERSION ?= v1.48.0
GO_VULN_CHECK_VERSION ?= latest
GOIMPORTS_VERSION ?= latest

# add ./hack/tools/bin to the PATH
export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

$(GO_VULN_CHECK):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/vuln/cmd/govulncheck@$(GO_VULN_CHECK_VERSION)

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOIMPORTS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
