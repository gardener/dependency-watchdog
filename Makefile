# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
#
# SPDX-License-Identifier: Apache-2.0

VERSION             := $(shell cat VERSION)
REGISTRY            := europe-docker.pkg.dev/gardener-project/public/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/dependency-watchdog
IMAGE_TAG           := $(VERSION)
BIN_DIR             := bin

# Tools
TOOLS_DIR := hack/tools
include hack/tools.mk
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
	@./hack/check.sh --golangci-lint-config=./.golangci.yaml ./controllers/... ./internal/...
	@./hack/check-imports.sh ./api/... ./cmd/... ./controllers/... ./internal/...

.PHONY: import-boss 
import-boss: $(GO_IMPORT_BOSS)
	@./hack/check-imports.sh ./cmd/...

.PHONY: format
format:
	@./hack/format.sh ./controllers ./internal

.PHONY: test
test: $(SETUP_ENVTEST) $(GOTESTFMT)
	@./hack/test.sh

.PHONY: kind-tests
kind-tests:
	@./hack/kind-test.sh

.PHONY: install-envtest
install-envtest: $(SETUP_ENVTEST)
	$(shell $(ENVTEST) --os $(go env GOOS) --arch $(go env GOARCH) --use-env use $(ENVTEST_K8S_VERSION) -p path)

.PHONY: verify
verify: check format test

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@./hack/addlicenseheaders.sh

# Taken this trick from https://pawamoy.github.io/posts/pass-makefile-args-as-typed-in-command-line/
args = $(foreach a,$($(subst -,_,$1)_args),$(if $(value $a),$a="$($a)"))
stress_args = test-package test-func tool-params

.PHONY: stress
stress: $(GO_STRESS)
	@./hack/stress-test.sh $@ $(call args,$@)

.PHONY: sast
sast: $(GOSEC)
	@./hack/sast.sh

.PHONY: sast-report
sast-report:$(GOSEC)
	@./hack/sast.sh --gosec-report true
