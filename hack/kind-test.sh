#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "> kind-test"
# to remove flakiness of the tests, the tests should not be run in parallel hence `-p 1` flag is passed 
# failfast test is set as with sequential run we need to fix the test once it fails and also aids in identifying flakiness
go test -failfast -v -p 1 --tags=kind_tests ./... -coverprofile cover.out
