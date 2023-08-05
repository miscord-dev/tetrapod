# Download zig for builarch amd64
FROM --platform=$BUILDPLATFORM golang:1.19 AS zigdownloader-amd64

RUN apt-get update && \
    apt-get install -y --no-install-recommends xz-utils && \
    curl -LO https://ziglang.org/download/0.11.0/zig-linux-x86_64-0.11.0.tar.xz && \
    tar xf zig-*.tar.xz && \
    mv ./zig-*/ /zig/

# Download zig for builarch arm64
FROM --platform=$BUILDPLATFORM golang:1.19 AS zigdownloader-arm64

RUN apt-get update && \
    apt-get install -y --no-install-recommends xz-utils && \
    curl -LO https://ziglang.org/download/0.11.0/zig-linux-aarch64-0.11.0.tar.xz && \
    tar xf zig-*.tar.xz && \
    mv ./zig-*/ /zig/

FROM zigdownloader-${BUILDARCH} AS zigdownloader

# Prepare for targetarch amd64
FROM --platform=$BUILDPLATFORM golang:1.20 AS gobase-amd64

ENV ZIGTARGET x86_64-linux-gnu

# Prepare for targetarch arm64
FROM --platform=$BUILDPLATFORM golang:1.20 AS gobase-arm64

ENV ZIGTARGET aarch64-linux-gnu

# Build the manager binary
FROM gobase-${TARGETARCH} AS gomod

ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY controlplane/ controlplane/
COPY tetrad/ tetrad/
COPY pkg/ pkg/
COPY disco/ disco/
COPY tetraengine/ tetraengine/

FROM gomod AS builder

COPY --from=zigdownloader /zig /zig

ENV PATH $PATH:/zig

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN mkdir -p bin && \
    GOOS=linux GOARCH=${TARGETARCH} CGO_ENABLED=1 CC="zig cc -target ${ZIGTARGET}" go build -a -o ./bin/tetrad ./tetrad && \
    GOOS=linux GOARCH=${TARGETARCH} CGO_ENABLED=0                                  go build -a -o ./bin/tetrad-entrypoint ./tetrad/cmd/tetrad-entrypoint

FROM gomod AS tetracni

ARG TARGETARCH

COPY tetracni/ tetracni/
COPY Makefile Makefile

RUN GOOS=linux GOARCH=${TARGETARCH} make cni-plugins

# Fetch CNI plugins
FROM golang:1.19 AS plugins

WORKDIR /workspace
COPY aqua.yaml aqua.yaml
COPY aqua-policy.yaml aqua-policy.yaml
COPY aqua/ aqua/
ENV AQUA_DISABLE_POLICY=true

RUN wget https://github.com/aquaproj/aqua/releases/latest/download/aqua_linux_$(go env GOARCH).tar.gz && \
    tar xf aqua_linux_*.tar.gz -C /bin/ && \
    mkdir -p /plugins && \
    aqua i && \
    cp $(aqua which bridge) /plugins/ && \
    cp $(aqua which host-local) /plugins/

FROM debian:bullseye

WORKDIR /
COPY tetracni/cni /config
COPY --from=builder /workspace/bin/tetrad-entrypoint .
COPY --from=builder /workspace/bin/tetrad .
COPY --from=tetracni /workspace/bin/* /plugins/
COPY --from=plugins /plugins/* /plugins/

ENTRYPOINT ["/tetrad-entrypoint", "/tetrad"]
