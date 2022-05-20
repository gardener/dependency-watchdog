# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the go source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager dwd.go


FROM alpine
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
