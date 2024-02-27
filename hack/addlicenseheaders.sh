#!/usr/bin/env bash
#

# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "> Adding Apache License header to all go files where it is not present"

addlicense \
  -f "hack/LICENSE_BOILERPLATE.txt" \
  -ignore "vendor/**" \
  -ignore "**/*.md" \
  -ignore "**/*.yaml" \
  -ignore "**/Dockerfile" \
  .
