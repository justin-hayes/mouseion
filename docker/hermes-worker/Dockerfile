# syntax=docker/dockerfile:1.7

ARG DEBIAN_VERSION=bookworm
ARG GO_VERSION=1.24.6
ARG NODE_MAJOR=22
ARG PROTOC_VERSION=29.3
ARG PROTOC_SHA256_AMD64=3e866620c5be27664f3d2fa2d656b5f3e09b5152b42f1bedbf427b333e90021a
ARG PROTOC_SHA256_ARM64=6427349140e01f06e049e707a58709a4f221ae73ab9a0425bc4a00c8d0e1ab32
ARG UV_VERSION=0.8.13
ARG OPENCODE_VERSION=1.18.26
ARG GH_VERSION=2.99.0
ARG GH_SHA256_AMD64=ed4960225d2833e04a61590d9fa2b5773d147f3aa375459e5466a40c102f3832
ARG GH_SHA256_ARM64=564eff56a61e8caf193efde16937fba879eb62a3a479c9dd6be2001e7647680b

# ---------------------------------------------------------------------------
# Go toolchain
# ---------------------------------------------------------------------------

FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS go-toolchain


# ---------------------------------------------------------------------------
# Runtime / dev image
# ---------------------------------------------------------------------------

FROM debian:${DEBIAN_VERSION}-slim

ARG TARGETARCH
ARG NODE_MAJOR
ARG PROTOC_VERSION
ARG PROTOC_SHA256_AMD64
ARG PROTOC_SHA256_ARM64
ARG UV_VERSION
ARG OPENCODE_VERSION

ENV DEBIAN_FRONTEND=noninteractive

# Basic development/runtime utilities.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        build-essential \
        ca-certificates \
        curl \
        file \
        git \
        gnupg \
        jq \
        less \
        openssh-client \
        procps \
        ripgrep \
        rsync \
        shellcheck \
        sqlite3 \
        unzip \
        xz-utils \
        zip \
    && rm -rf /var/lib/apt/lists/*


# ---------------------------------------------------------------------------
# Node.js
# ---------------------------------------------------------------------------

RUN install -d -m 0755 /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
       | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo \
       "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
       > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && corepack enable


# ---------------------------------------------------------------------------
# Go
#
# Copy directly from the official Go image rather than installing Go via apt.
# ---------------------------------------------------------------------------

COPY --from=go-toolchain /usr/local/go /usr/local/go

ENV PATH="/usr/local/go/bin:${PATH}"


# ---------------------------------------------------------------------------
# Python + uv
#
# Debian supplies Python; uv handles project environments/dependencies.
# ---------------------------------------------------------------------------

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        python3 \
        python3-dev \
        python3-venv \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/${UV_VERSION}/install.sh \
    | env UV_INSTALL_DIR=/usr/local/bin sh


# ---------------------------------------------------------------------------
# protoc
# ---------------------------------------------------------------------------

RUN case "${TARGETARCH}" in \
        amd64) PROTOC_ARCH=x86_64; protoc_sha="${PROTOC_SHA256_AMD64}" ;; \
        arm64) PROTOC_ARCH=aarch_64; protoc_sha="${PROTOC_SHA256_ARM64}" ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /tmp/protoc.zip \
       "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${PROTOC_ARCH}.zip" \
    && printf '%s  %s\n' "${protoc_sha}" /tmp/protoc.zip | sha256sum -c - \
    && unzip -q /tmp/protoc.zip -d /usr/local \
    && rm /tmp/protoc.zip


# ---------------------------------------------------------------------------
# protobuf Go generators (required by `make gen`, pinned to repo versions)
# ---------------------------------------------------------------------------

RUN /usr/local/go/bin/go version >/dev/null 2>&1 \
    && GOBIN=/usr/local/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
    && GOBIN=/usr/local/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
    && rm -rf /root/.cache/go-build


# ---------------------------------------------------------------------------
# GitHub CLI
# ---------------------------------------------------------------------------

ARG GH_VERSION
ARG GH_SHA256_AMD64
ARG GH_SHA256_ARM64

RUN case "${TARGETARCH}" in \
        amd64) gh_suffix=amd64; gh_checksum="${GH_SHA256_AMD64}" ;; \
        arm64) gh_suffix=arm64; gh_checksum="${GH_SHA256_ARM64}" ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /tmp/gh.tar.gz \
       "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_suffix}.tar.gz" \
    && printf '%s  %s\n' "${gh_checksum}" /tmp/gh.tar.gz | sha256sum -c - \
    && tar -xzf /tmp/gh.tar.gz -C /tmp \
    && install -m 0755 "/tmp/gh_${GH_VERSION}_linux_${gh_suffix}/bin/gh" /usr/local/bin/gh \
    && rm -rf /tmp/gh.tar.gz "/tmp/gh_${GH_VERSION}_linux_${gh_suffix}"


# ---------------------------------------------------------------------------
# OpenCode
#
# npm is preferable here to curl | bash because we can pin the version.
# OpenCode officially supports npm installation.
# ---------------------------------------------------------------------------

RUN npm install --global "opencode-ai@${OPENCODE_VERSION}" \
    && npm cache clean --force

# OpenCode currently documents npm install -g opencode-ai as an official
# installation mechanism.


# ---------------------------------------------------------------------------
# Runtime user
# ---------------------------------------------------------------------------

ARG USERNAME=hermes
ARG USER_UID=10000
ARG USER_GID=10000

RUN groupadd \
        --gid "${USER_GID}" \
        "${USERNAME}" \
    && useradd \
        --uid "${USER_UID}" \
        --gid "${USER_GID}" \
        --create-home \
        --shell /bin/bash \
        "${USERNAME}"


# ---------------------------------------------------------------------------
# Writable runtime locations
# ---------------------------------------------------------------------------

RUN mkdir -p \
        /workspace \
        /home/${USERNAME}/.cache \
        /home/${USERNAME}/.config \
        /home/${USERNAME}/.local/share \
        /home/${USERNAME}/go \
    && chown -R "${USERNAME}:${USERNAME}" \
        /workspace \
        /home/${USERNAME}


ENV HOME="/home/${USERNAME}"

ENV XDG_CONFIG_HOME="${HOME}/.config"
ENV XDG_CACHE_HOME="${HOME}/.cache"
ENV XDG_DATA_HOME="${HOME}/.local/share"

ENV GOPATH="${HOME}/go"
ENV GOMODCACHE="${HOME}/go/pkg/mod"
ENV GOCACHE="${HOME}/.cache/go-build"

ENV PNPM_HOME="${HOME}/.local/share/pnpm"
ENV PATH="${PNPM_HOME}:${GOPATH}/bin:${HOME}/.local/bin:${PATH}"

# Keep temporary build state somewhere the runtime user owns.
ENV TMPDIR="/tmp"


# ---------------------------------------------------------------------------
# Git defaults suitable for an autonomous coding environment
# ---------------------------------------------------------------------------

RUN git config --system init.defaultBranch main \
    && git config --system core.autocrlf false \
    && git config --system advice.detachedHead false


# ---------------------------------------------------------------------------
# Smoke test the immutable toolchain at image-build time
# ---------------------------------------------------------------------------

RUN set -eux; \
    git --version; \
    gh --version; \
    go version; \
    node --version; \
    npm --version; \
    corepack --version; \
    python3 --version; \
    uv --version; \
    protoc --version; \
    protoc-gen-go --version; \
    protoc-gen-go-grpc --version; \
    opencode --version; \
    rg --version


USER ${USERNAME}
WORKDIR /workspace

CMD ["bash"]
