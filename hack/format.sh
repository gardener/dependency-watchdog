#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "> Format"

# shellcheck disable=SC2068
goimports -l -w $@
