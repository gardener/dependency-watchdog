# This make file is supposed to be included in the top-level make file.

SYSTEM_NAME       := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SYSTEM_ARCH       := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
TOOLS_DIR         := hack/tools
TOOLS_BIN_DIR     := $(TOOLS_DIR)/bin
GOLANGCI_LINT     := $(TOOLS_BIN_DIR)/golangci-lint
GO_VULN_CHECK     := $(TOOLS_BIN_DIR)/govulncheck
GOIMPORTS         := $(TOOLS_BIN_DIR)/goimports
LOGCHECK          := $(TOOLS_BIN_DIR)/logcheck.so # plugin binary
GO_ADD_LICENSE    := $(TOOLS_BIN_DIR)/addlicense
GO_IMPORT_BOSS    := $(TOOLS_BIN_DIR)/import-boss
GO_STRESS         := $(TOOLS_BIN_DIR)/stress
SETUP_ENVTEST     := $(TOOLS_BIN_DIR)/setup-envtest
GOSEC             := $(TOOLS_BIN_DIR)/gosec

# Use this function to get the version of a go module from go.mod
version_gomod = $(shell go list -mod=mod -f '{{ .Version }}' -m $(1))

#default tool versions
GOLANGCI_LINT_VERSION ?= v2.0.2
GO_VULN_CHECK_VERSION ?= latest
GOIMPORTS_VERSION ?= latest
LOGCHECK_VERSION ?= ee13c7d8519f930e352785de176d09d75e65027c # this commit hash corresponds to v1.115.2 which is the gardener/gardener version in go.mod - we could use regular tags when https://github.com/gardener/gardener/issues/8811 is resolved
GO_ADD_LICENSE_VERSION ?= latest
# k8s version is required as import-boss is part of the kubernetes/kubernetes repository.
K8S_VERSION ?= $(subst v0,v1,$(call version_gomod,k8s.io/api))
GO_STRESS_VERSION ?= latest
CONTROLLER_RUNTIME_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-runtime)
GOSEC_VERSION ?= v2.21.4

# add ./hack/tools/bin to the PATH
export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

$(GO_VULN_CHECK):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/vuln/cmd/govulncheck@$(GO_VULN_CHECK_VERSION)

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOIMPORTS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(LOGCHECK):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/gardener/gardener/hack/tools/logcheck@$(LOGCHECK_VERSION)

$(GO_ADD_LICENSE):
    GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/addlicense@$(GO_ADD_LICENSE_VERSION)

$(GO_IMPORT_BOSS):
	mkdir -p hack/tools/bin/work/import-boss
	curl -L -o hack/tools/bin/work/import-boss/main.go https://raw.githubusercontent.com/kubernetes/kubernetes/$(K8S_VERSION)/cmd/import-boss/main.go
	go build -o $(TOOLS_BIN_DIR) ./hack/tools/bin/work/import-boss

$(GO_STRESS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/tools/cmd/stress@$(GO_STRESS_VERSION)

$(SETUP_ENVTEST):
	curl -Lo $(SETUP_ENVTEST) https://github.com/kubernetes-sigs/controller-runtime/releases/download/$(CONTROLLER_RUNTIME_VERSION)/setup-envtest-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(SETUP_ENVTEST)

$(GOSEC):
	GOSEC_VERSION=$(GOSEC_VERSION) bash $(TOOLS_DIR)/install-gosec.sh
