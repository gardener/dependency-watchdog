#!/usr/bin/env bash
#
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
