#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
GOARCH=${GOARCH:-$(go env GOARCH)}
PLATFORM=${PLATFORM:-linux/${GOARCH}}

function build_docker_image() {
  local version="$(cat "${REPO_ROOT}/VERSION")"
  printf '%s\n' "Building dependency-watchdog:${version} with:
    GOARCH=${GOARCH}, PLATFORM=${PLATFORM}"

  docker buildx build \
    --platform "${PLATFORM}" \
    --build-arg VERSION="${version}" \
    --tag dependency-watchdog-${GOARCH}:${version} \
    --file ${REPO_ROOT}/Dockerfile \
    ${REPO_ROOT} # docker context is the root of the repository
}

build_docker_image