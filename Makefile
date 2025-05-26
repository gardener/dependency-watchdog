# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR            := $(REPO_ROOT)/hack
VERSION             := $(shell cat VERSION)
REGISTRY            := europe-docker.pkg.dev/gardener-project/public/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/dependency-watchdog
IMAGE_TAG           := $(VERSION)
BIN_DIR             := bin

# Tools
TOOLS_DIR := $(HACK_DIR)/tools
include $(HACK_DIR)/tools.mk
ENVTEST   := $(TOOLS_BIN_DIR)/setup-envtest

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make tidy

.PHONY: check-vulnerabilities
check-vulnerabilities: $(GO_VULN_CHECK)
	$(GO_VULN_CHECK) ./...

.PHONY: build
build:
	@.ci/build

.PHONY: build-local
build-local:
	@env LOCAL_BUILD=1 .ci/build

.PHONY: docker-image
docker-image:
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) -f Dockerfile --rm .

# Builds the docker image for the specified platform.
# Usage: make docker-build-<arch>
# Example: make docker-build-amd64, make docker-build-arm64
.PHONY: docker-build-%
docker-build-%:
	@GOARCH=$$(echo $* | cut -d- -f 1) hack/docker-build.sh


.PHONY: docker-push
docker-push:
	@if ! docker images $(IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(IMAGE_TAG); then echo "$(IMAGE_REPOSITORY) version $(IMAGE_TAG) is not yet built. Please run 'make docker-image'"; false; fi
	@docker push $(IMAGE_REPOSITORY):$(IMAGE_TAG)

.PHONY: clean
clean:
	@rm -rf $(BIN_DIR)/
	@rm -rf $(TOOLS_BIN_DIR)

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT) $(LOGCHECK) $(GO_IMPORT_BOSS)
	@$(HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./controllers/... ./internal/...
	@$(HACK_DIR)/check-imports.sh ./api/... ./cmd/... ./controllers/... ./internal/...

.PHONY: import-boss 
import-boss: $(GO_IMPORT_BOSS)
	@$(HACK_DIR)/check-imports.sh ./cmd/...

.PHONY: format
format:
	@$(HACK_DIR)/format.sh ./controllers ./internal

.PHONY: test
test: $(SETUP_ENVTEST)
	@$(HACK_DIR)/test.sh

.PHONY: kind-tests
kind-tests:
	@$(HACK_DIR)/kind-test.sh

.PHONY: install-envtest
install-envtest: $(SETUP_ENVTEST)
	$(shell $(ENVTEST) --os $(go env GOOS) --arch $(go env GOARCH) --use-env use $(ENVTEST_K8S_VERSION) -p path)

.PHONY: verify
verify: check format test

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@$(HACK_DIR)/addlicenseheaders.sh

# Taken this trick from https://pawamoy.github.io/posts/pass-makefile-args-as-typed-in-command-line/
args = $(foreach a,$($(subst -,_,$1)_args),$(if $(value $a),$a="$($a)"))
stress_args = test-package test-func tool-params

.PHONY: stress
stress: $(GO_STRESS)
	@$(HACK_DIR)/stress-test.sh $@ $(call args,$@)

.PHONY: sast
sast: $(GOSEC)
	@$(HACK_DIR)/sast.sh

.PHONY: sast-report
sast-report:$(GOSEC)
	@$(HACK_DIR)/sast.sh --gosec-report true
