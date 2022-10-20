# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
#
# SPDX-License-Identifier: Apache-2.0

VERSION             := $(shell cat VERSION)
REGISTRY            := eu.gcr.io/gardener-project/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/dependency-watchdog
IMAGE_TAG           := $(VERSION)
BIN_DIR             := bin

# Tools
TOOLS_DIR := hack/tools
include hack/tools.mk

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
check: $(GOIMPORTS) $(GOLANGCI_LINT) $(GOMEGACHECK) $(LOGCHECK)
	@./hack/check.sh --golangci-lint-config=./.golangci.yaml ./controllers/... ./internal/...

.PHONY: format
format:
	@./hack/format.sh ./controllers ./internal

.PHONY: test
test:
	go test ./... -coverprofile cover.out

.PHONY: verify
verify: check format test

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@./hack/addlicenseheaders.sh
