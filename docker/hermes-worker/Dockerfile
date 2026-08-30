# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24.6
FROM golang:${GO_VERSION}-bookworm AS go-toolchain

FROM nikolaik/python-nodejs:python3.11-nodejs20@sha256:8f958bdc1b4a422bfafd97cab4f69836401f616ae985d4b57a53d254f5bcb038

ARG TARGETARCH

ARG PROTOC_VERSION=29.3
ARG PROTOC_SHA256_AMD64=3e866620c5be27664f3d2fa2d656b5f3e09b5152b42f1bedbf427b333e90021a
ARG PROTOC_SHA256_ARM64=6427349140e01f06e049e707a58709a4f221ae73ab9a0425bc4a00c8d0e1ab32

ARG CODEX_VERSION=0.150.1

USER root

# Runtime layout:
#
#   /workspace
#     Repository mount point used by Hermes.
#
#   /usr/local/go
#     Go toolchain copied from the official Go image.
#
#   /opt/go/bin
#     Additional Go tooling installed into the image.
#
#   /home/hermes
#     Conventional runtime HOME.
#
# Hermes starts the container using the host account's numeric UID/GID via
# docker_run_as_host_user=true. The image therefore remains generic and does
# not bake in a host-specific UID or GID.
#
# CODEX_HOME is declared explicitly so it unambiguously matches where Hermes
# bind-mounts the host's ~/.codex config and auth (see the mkdir/chmod below).
# It must equal the container path used in terminal.docker_volumes, otherwise
# Codex silently reads an empty home and reports "Not logged in".
ENV GOPATH=/opt/go \
    GOBIN=/opt/go/bin \
    HOME=/home/hermes \
    CODEX_HOME=/home/hermes/.codex \
    TMPDIR=/workspace/.tmp \
    PATH=/usr/local/go/bin:/opt/go/bin:/usr/local/bin:/usr/bin:/bin

COPY --from=go-toolchain /usr/local/go /usr/local/go

# General development tooling required by Mouseion and the Hermes worker.
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

# Verify that the copied Go toolchain is present and reachable through PATH.
RUN set -eux; \
    test -x /usr/local/go/bin/go; \
    command -v go; \
    go version

# Install a pinned protoc release instead of relying on Debian's older
# protobuf compiler. The archive also contains protobuf's standard includes.
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
        amd64) \
            protoc_arch=x86_64; \
            protoc_sha="${PROTOC_SHA256_AMD64}" \
            ;; \
        arm64) \
            protoc_arch=aarch_64; \
            protoc_sha="${PROTOC_SHA256_ARM64}" \
            ;; \
        *) \
            echo "Unsupported TARGETARCH=${TARGETARCH}" >&2; \
            exit 1 \
            ;; \
    esac; \
    curl -fsSL --retry 3 --retry-all-errors \
        -o /tmp/protoc.zip \
        "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${protoc_arch}.zip"; \
    printf '%s  %s\n' "${protoc_sha}" /tmp/protoc.zip | sha256sum -c -; \
    mkdir -p /tmp/protoc; \
    unzip -q /tmp/protoc.zip -d /tmp/protoc; \
    install -m 0755 /tmp/protoc/bin/protoc /usr/local/bin/protoc; \
    cp -a /tmp/protoc/include/. /usr/local/include/; \
    rm -rf /tmp/protoc /tmp/protoc.zip; \
    protoc --version

# Install the protobuf Go generators required by the repository.
RUN set -eux; \
    mkdir -p /opt/go/bin /workspace/.tmp; \
    GOBIN=/opt/go/bin \
        go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12; \
    GOBIN=/opt/go/bin \
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1; \
    rm -rf /opt/go/pkg /root/.cache/go-build; \
    chmod -R a+rX /opt/go; \
    command -v protoc-gen-go; \
    command -v protoc-gen-go-grpc

# Install Codex globally into immutable image space.
#
# The executable is independent of ~/.codex, so individual config/auth files
# can be bind-mounted without shadowing the CLI installation.
RUN set -eux; \
    npm install --global "@openai/codex@${CODEX_VERSION}"; \
    command -v codex; \
    codex --version

# Create runtime directories.
#
# The runtime UID/GID is supplied by Hermes and may not correspond to an
# /etc/passwd entry inside the image, so these locations must be writable by
# an arbitrary numeric user.
#
# Hermes is expected to bind-mount selected Codex files into:
#
#   /home/hermes/.codex/auth.json
#   /home/hermes/.codex/config.toml
#   /home/hermes/.codex/fast.config.toml
#   /home/hermes/.codex/normal.config.toml
#   /home/hermes/.codex/deep.config.toml
RUN set -eux; \
    mkdir -p \
        /workspace \
        /workspace/.tmp \
        /home/hermes/.codex \
        /home/hermes/.cache \
        /home/hermes/.config \
        /home/hermes/.local; \
    chmod 0777 /workspace; \
    chmod 0777 /workspace/.tmp; \
    chmod 0777 /home/hermes; \
    chmod 0777 /home/hermes/.codex; \
    chmod 0777 /home/hermes/.cache; \
    chmod 0777 /home/hermes/.config; \
    chmod 0777 /home/hermes/.local

# Fail the image build immediately if essential tools are not available on
# the final configured PATH.
RUN set -eux; \
    echo "PATH=${PATH}"; \
    command -v go; \
    go version; \
    command -v codex; \
    codex --version; \
    command -v protoc; \
    protoc --version; \
    command -v protoc-gen-go; \
    command -v protoc-gen-go-grpc

# Do not set a final USER.
#
# Hermes supplies the host user's numeric UID/GID at runtime using:
#
#   docker_run_as_host_user: true
#
# This preserves correct ownership for files written to /workspace while
# allowing the same GHCR image to run on hosts with different UIDs/GIDs.
WORKDIR /workspace
