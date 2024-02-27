#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

shift # first argument will be the name of the command which we are not interested in, so ignoring it
for p in "$@" ; do
  IFS='=' read -r key val <<< "$p"
  case $key in
   test-package)
    package="$val"
      ;;
   test-func)
    func="$val"
    ;;
   tool-params)
     params="$val"
    ;;
  esac
done

# compile test binary
rm -f /tmp/pkg-stress.test
go test -c "$package" -o /tmp/pkg-stress.test
chmod +x /tmp/pkg-stress.test

# run the stress tool
stress $params  /tmp/pkg-stress.test -test.run=$func
