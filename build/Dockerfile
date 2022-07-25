# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

From golang:1.18.3 as builder

WORKDIR /go/src/github.com/gardener/dependency-watchdog
COPY . .

RUN make build

FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /go/src/github.com/gardener/dependency-watchdog/bin/dependency-watchdog /dependency-watchdog
WORKDIR /
ENTRYPOINT ["/dependency-watchdog"]
