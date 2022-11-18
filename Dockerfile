FROM golang:1.19.2 AS builder

WORKDIR /go/src/github.com/gardener/dependency-watchdog
COPY . .

#build
RUN make build

FROM gcr.io/distroless/static-debian11:nonroot AS dependency-watchdog

COPY --from=builder /go/src/github.com/gardener/dependency-watchdog/bin/dependency-watchdog /dependency-watchdog
WORKDIR /
ENTRYPOINT ["/dependency-watchdog"]