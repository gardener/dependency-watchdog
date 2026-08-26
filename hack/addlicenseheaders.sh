#!/usr/bin/env bash
#

# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "> Adding Apache License header to all go files where it is not present"

YEAR="$(date +%Y)"

temp_file=$(mktemp)
trap "rm -f $temp_file" EXIT
sed -e "s/YEAR/${YEAR}/g" -e 's|^// *||' hack/boilerplate.go.txt > $temp_file

addlicense \
  -f $temp_file \
  -ignore "**/*.md" \
  -ignore "**/*.yaml" \
  -ignore "**/*.yml" \
  -ignore "**/Dockerfile" \
  .
