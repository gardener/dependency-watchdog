# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
#
# SPDX-License-Identifier: Apache-2.0

VERSION             := $(shell cat VERSION)
REGISTRY            := eu.gcr.io/gardener-project/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/dependency-watchdog
IMAGE_TAG           := $(VERSION)
BUILD_DIR           := build
BIN_DIR             := bin

.PHONY: revendor
revendor:
	@env GO111MODULE=on go mod vendor -v
	@env GO111MODULE=on go mod tidy -v

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor

.PHONY: build
build: 
	@.ci/build

.PHONY: build-local
build-local:
	@env LOCAL_BUILD=1 .ci/build

.PHONY: docker-image
docker-image: 
	@if [[ ! -f $(BIN_DIR)/linux-amd64/dependency-watchdog ]]; then echo "No binary found. Please run 'make build'"; false; fi
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

.PHONY: test
test:
	@.ci/test

.PHONY: verify
verify: check test