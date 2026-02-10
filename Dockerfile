FROM golang:1.26.0 AS builder

WORKDIR /go/src/github.com/gardener/dependency-watchdog
COPY . .

#build
RUN make build

FROM gcr.io/distroless/static-debian12:nonroot AS dependency-watchdog

COPY --from=builder /go/src/github.com/gardener/dependency-watchdog/bin/dependency-watchdog /usr/local/bin/dependency-watchdog
WORKDIR /
ENTRYPOINT ["/usr/local/bin/dependency-watchdog"]