# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the go source
COPY internal ./internal
COPY vendor ./vendor
COPY cmd ./cmd
COPY controllers ./controllers
COPY go.mod go.sum ./
COPY *.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dwd -ldflags -a .

FROM alpine:3.15.4
WORKDIR /
COPY --from=builder /workspace/dwd .

ENTRYPOINT ["./dwd"]
