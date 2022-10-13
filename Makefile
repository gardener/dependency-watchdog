# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_DIR := hack/tools
TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin
GO_VULN_CHECK              := $(TOOLS_BIN_DIR)/govulncheck
VERSION             := $(shell cat VERSION)
REGISTRY            := eu.gcr.io/gardener-project/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/dependency-watchdog
IMAGE_TAG           := $(VERSION)
BUILD_DIR           := build
BIN_DIR             := bin

# default tool versions
GO_VULN_CHECK_VERSION ?= latest

$(GO_VULN_CHECK):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/vuln/cmd/govulncheck@$(GO_VULN_CHECK_VERSION)

.PHONY: revendor
revendor:
	@env GO111MODULE=on go mod tidy -v
	@env GO111MODULE=on go mod vendor -v

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor

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
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) -f $(BUILD_DIR)/Dockerfile --rm .

.PHONY: docker-push
docker-push:
	@if ! docker images $(IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(IMAGE_TAG); then echo "$(IMAGE_REPOSITORY) version $(IMAGE_TAG) is not yet built. Please run 'make docker-image'"; false; fi
	@docker push $(IMAGE_REPOSITORY):$(IMAGE_TAG)

.PHONY: clean
clean:
	@rm -rf $(BIN_DIR)/

.PHONY: check
check:
	@.ci/check

.PHONY: format
format:
	@./vendor/github.com/gardener/gardener/hack/format.sh ./cmd ./pkg

.PHONY: test
test:
	@.ci/test

.PHONY: verify
verify: check test