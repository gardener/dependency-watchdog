# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.15

RUN apk add --update bash curl

COPY ./bin/linux-amd64/dependency-watchdog /usr/local/bin/dependency-watchdog
WORKDIR /
ENTRYPOINT ["/usr/local/bin/dependency-watchdog"]
