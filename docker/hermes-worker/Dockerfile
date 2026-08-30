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
#     Repository mount point.
#
#   /opt/go
#     Immutable Go tooling installed into the image.
#
#   /usr/local/bin/codex
#     Immutable Codex executable installed into the image.
#
#   /tmp/hermes-home
#     Disposable HOME for the arbitrary numeric UID/GID supplied by Hermes.
#
# Codex therefore uses the standard:
#
#   $HOME/.codex
#
# location. Selected files such as config.toml, auth.json, and the Codex
# profile config files are mounted individually by Hermes at runtime.
ENV GOPATH=/opt/go \
    GOBIN=/opt/go/bin \
    HOME=/tmp/hermes-home \
    PATH=/usr/local/go/bin:/opt/go/bin:/usr/local/bin:${PATH}

COPY --from=go-toolchain /usr/local/go /usr/local/go

# General development tooling needed by Mouseion and the agent environment.
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

# Install a pinned protoc release instead of relying on Debian's older
# protobuf compiler. The archive also includes protobuf's well-known types.
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
    rm -rf /tmp/protoc /tmp/protoc.zip

# Install the protobuf Go generators required by the repository.
#
# These are immutable image tools. Runtime module/build caches should live
# under HOME rather than requiring /opt/go to be writable.
RUN set -eux; \
    mkdir -p /opt/go/bin; \
    GOBIN=/opt/go/bin \
        go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12; \
    GOBIN=/opt/go/bin \
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1; \
    rm -rf /opt/go/pkg /root/.cache/go-build; \
    chmod -R a+rX /opt/go

# Install Codex into a temporary location, then copy only the executable into
# immutable image space.
#
# This keeps the binary separate from ~/.codex so mounting Codex configuration
# files at runtime cannot shadow or replace the executable.
RUN set -eux; \
    mkdir -p /tmp/codex-install/bin /tmp/codex-install/home; \
    curl -fsSL --retry 3 --retry-all-errors \
        https://chatgpt.com/codex/install.sh \
        | CODEX_HOME=/tmp/codex-install/home \
          CODEX_INSTALL_DIR=/tmp/codex-install/bin \
          CODEX_NON_INTERACTIVE=true \
          sh -s -- --release "${CODEX_VERSION}"; \
    test -x /tmp/codex-install/bin/codex; \
    install -m 0755 /tmp/codex-install/bin/codex /usr/local/bin/codex; \
    rm -rf /tmp/codex-install; \
    command -v codex; \
    codex --version

# Create the runtime directories expected by Hermes and Codex.
#
# Hermes launches this container with docker_run_as_host_user=true, which means
# the runtime UID/GID may not correspond to any user in /etc/passwd.
#
# A writable synthetic HOME avoids relying on the base image's `pn` account.
#
# ~/.codex is created separately because Hermes will bind-mount individual
# files into it:
#
#   auth.json
#   config.toml
#   fast.config.toml
#   normal.config.toml
#   deep.config.toml
#
# Any other Codex state remains local to the disposable worker.
RUN set -eux; \
    mkdir -p \
        /workspace \
        /tmp/hermes-home/.codex; \
    chmod 0777 /workspace; \
    chmod 1777 /tmp/hermes-home; \
    chmod 0777 /tmp/hermes-home/.codex

# Do not set a final USER.
#
# Hermes supplies the host account's numeric UID/GID at runtime via:
#
#   docker_run_as_host_user: true
#
# This keeps files created in the mounted repository owned by the host Hermes
# account while avoiding assumptions about the base image's `pn` UID/GID.
WORKDIR /workspace
