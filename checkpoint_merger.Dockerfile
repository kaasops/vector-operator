# Build the checkpoint-merger binary
FROM golang:1.22 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION="dev"

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/checkpoint_merger/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags="-X github.com/kaasops/vector-operator/internal/buildinfo.Version=${VERSION}" \
    -a -o checkpoint-merger cmd/main.go

# The init container writes into the vector data dir (a hostPath owned by the
# vector agent, which runs as root), so no nonroot user here.
FROM gcr.io/distroless/static:latest
WORKDIR /
COPY --from=builder /workspace/checkpoint-merger .

ENTRYPOINT ["/checkpoint-merger"]