# This make file is supposed to be included in the top-level make file.

TOOLS_DIR := hack/tools
TOOLS_BIN_DIR     := $(TOOLS_DIR)/bin
GOLANGCI_LINT     := $(TOOLS_BIN_DIR)/golangci-lint
GO_VULN_CHECK     := $(TOOLS_BIN_DIR)/govulncheck
GOIMPORTS         := $(TOOLS_BIN_DIR)/goimports
LOGCHECK          := $(TOOLS_BIN_DIR)/logcheck.so # plugin binary
GO_ADD_LICENSE    := $(TOOLS_BIN_DIR)/addlicense
GO_IMPORT_BOSS    := $(TOOLS_BIN_DIR)/import-boss
GO_STRESS         := $(TOOLS_BIN_DIR)/stress
SETUP_ENVTEST     := $(TOOLS_BIN_DIR)/setup-envtest

#default tool versions
GOLANGCI_LINT_VERSION ?= v1.55.2
GO_VULN_CHECK_VERSION ?= latest
GOIMPORTS_VERSION ?= latest
LOGCHECK_VERSION ?= cee120166b7b8a857dea90fc0217d86c2e41850f # this commit hash corresponds to v1.86.0 which is the gardener/gardener version in go.mod - we could use regular tags when https://github.com/gardener/gardener/issues/8811 is resolved
GO_ADD_LICENSE_VERSION ?= latest
# import boss failing with latest , so pinning to most up-to-date successfull version
# refer https://pkg.go.dev/k8s.io/code-generator@v0.26.3/cmd/import-boss?tab=versions for list of versions
GO_IMPORT_BOSS_VERSION ?= v0.28.4 
GO_STRESS_VERSION ?= latest
SETUP_ENVTEST_VERSION ?= latest

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

$(LOGCHECK):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/gardener/gardener/hack/tools/logcheck@$(LOGCHECK_VERSION)

$(GO_ADD_LICENSE):
    GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/addlicense@$(GO_ADD_LICENSE_VERSION)

$(GO_IMPORT_BOSS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install k8s.io/code-generator/cmd/import-boss@$(GO_IMPORT_BOSS_VERSION)

$(GO_STRESS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/tools/cmd/stress@$(GO_STRESS_VERSION)

$(SETUP_ENVTEST):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)
