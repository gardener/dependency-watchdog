FROM golang:1.18.3 as builder

WORKDIR /go/src/github.com/gardener/dependency-watchdog
COPY . .

#build
RUN make build

FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /go/src/github.com/gardener/dependency-watchdog/bin/linux-amd64/dependency-watchdog /dependency-watchdog
WORKDIR /
ENTRYPOINT ["/dependency-watchdog"]