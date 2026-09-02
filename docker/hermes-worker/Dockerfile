# syntax=docker/dockerfile:1.7

ARG DEBIAN_VERSION=bookworm
ARG GO_VERSION=1.24.6
ARG NODE_MAJOR=22
ARG PROTOC_VERSION=29.3
ARG UV_VERSION=0.8.13
ARG OPENCODE_VERSION=latest

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
        amd64) PROTOC_ARCH=x86_64 ;; \
        arm64) PROTOC_ARCH=aarch_64 ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /tmp/protoc.zip \
       "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${PROTOC_ARCH}.zip" \
    && unzip -q /tmp/protoc.zip -d /usr/local \
    && rm /tmp/protoc.zip


# ---------------------------------------------------------------------------
# GitHub CLI
# ---------------------------------------------------------------------------

RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
       -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo \
       "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
       > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*


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
    opencode --version; \
    rg --version


USER ${USERNAME}
WORKDIR /workspace

CMD ["bash"]
