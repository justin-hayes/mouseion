# syntax=docker/dockerfile:1
#
ARG GO_VERSION=1.24.6
FROM golang:${GO_VERSION}-bookworm AS go-toolchain

# This is a disposable Hermes terminal image, not the Mouseion app image; the
# workspace is mounted by Hermes at /workspace at runtime. It is based on the
# same Python 3.11 + Node 20 image that Hermes uses for its docker backend by
# default, plus the build tooling needed by the Mouseion repository. We keep
# the base image's non-root `pn` user (uid 1000) as the runtime user: its uid
# matches the default host user on most dev machines, so a host-mounted
# /workspace stays writable without relying on a uid mapping workaround.
FROM nikolaik/python-nodejs:python3.11-nodejs20@sha256:8f958bdc1b4a422bfafd97cab4f69836401f616ae985d4b57a53d254f5bcb038

ARG TARGETARCH
ARG PROTOC_VERSION=29.3
ARG PROTOC_SHA256_AMD64=3e866620c5be27664f3d2fa2d656b5f3e09b5152b42f1bedbf427b333e90021a
ARG PROTOC_SHA256_ARM64=6427349140e01f06e049e707a58709a4f221ae73ab9a0425bc4a00c8d0e1ab32
ARG CODEX_VERSION=0.150.1

USER root

ENV GOPATH=/opt/go \
    GOBIN=/opt/go/bin \
    CODEX_HOME=/opt/codex/home \
    PATH=/usr/local/go/bin:/opt/go/bin:/opt/codex/bin:${PATH}

COPY --from=go-toolchain /usr/local/go /usr/local/go

# Keep the package set small while making the repository's normal build and
# Git workflows work without installing anything into the session.
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        build-essential \
        ca-certificates \
        cmake \
        curl \
        git \
        jq \
        openssh-client \
        pkg-config \
        ripgrep \
        unzip \
    && rm -rf /var/lib/apt/lists/*

# Install the pinned protoc release rather than relying on the distribution's
# older protobuf compiler. The release archive also provides the well-known
# protobuf include files required by protoc.
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
        amd64) protoc_arch=x86_64; protoc_sha="${PROTOC_SHA256_AMD64}" ;; \
        arm64) protoc_arch=aarch_64; protoc_sha="${PROTOC_SHA256_ARM64}" ;; \
        *) echo "Unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL --retry 3 --retry-all-errors \
        -o /tmp/protoc.zip \
        "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${protoc_arch}.zip"; \
    printf '%s  %s\n' "${protoc_sha}" /tmp/protoc.zip | sha256sum -c -; \
    mkdir /tmp/protoc; \
    unzip -q /tmp/protoc.zip -d /tmp/protoc; \
    install -m 0755 /tmp/protoc/bin/protoc /usr/local/bin/protoc; \
    cp -a /tmp/protoc/include/. /usr/local/include/; \
    rm -rf /tmp/protoc /tmp/protoc.zip

# Match the versions used by go.mod and Makefile. GOPATH is writable by the
# runtime user so later Go commands can use a disposable module/build cache.
RUN mkdir -p /opt/go/bin \
    && GOBIN=/opt/go/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
    && GOBIN=/opt/go/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
    && rm -rf /opt/go/pkg /root/.cache/go-build \
    && chown -R pn:pn /opt/go

# Install Codex without credentials. The release archive is placed in the
# image, while CODEX_HOME is populated at runtime from a host ~/.codex mounted
# over /opt/codex/home (see README). No profile configs or credentials are
# baked in.
RUN mkdir -p /opt/codex/bin /opt/codex/home \
    && chown -R pn:pn /opt/codex

RUN mkdir -p /workspace && chown pn:pn /workspace

USER pn

RUN curl -fsSL --retry 3 --retry-all-errors https://chatgpt.com/codex/install.sh \
    | CODEX_HOME=/opt/codex/home \
      CODEX_INSTALL_DIR=/opt/codex/bin \
      CODEX_NON_INTERACTIVE=true \
      sh -s -- --release "${CODEX_VERSION}"

WORKDIR /workspace
