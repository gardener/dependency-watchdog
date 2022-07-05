# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

From golang:1.18.3 as builder

WORKDIR /go/src/github.com/gardener/dependency-watchdog
COPY . .

RUN make build

FROM alpine:3.15.4

RUN apk add --update bash curl

COPY --from=builder /go/src/github.com/gardener/dependency-watchdog/bin/dependency-watchdog /usr/local/bin/dependency-watchdog
WORKDIR /
ENTRYPOINT ["/usr/local/bin/dependency-watchdog"]
