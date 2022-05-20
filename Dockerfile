# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the go source
COPY . .

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager dwd.go


FROM alpine
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
