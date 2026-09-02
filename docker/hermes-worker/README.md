# Mouseion Hermes worker image

This is a disposable Docker terminal image for [Hermes Agent](https://hermes-agent.nousresearch.com/docs/user-guide/docker). It is based on `debian:bookworm-slim` and provides the tools used by Mouseion agent sessions. Hermes runs the container as the host user's uid/gid (`docker_run_as_host_user`), so the image is uid-agnostic: it pre-creates world-writable runtime locations and ships a non-root `hermes` fallback user.

The installed toolchain is:

- Go 1.24.6
- Node 22, npm, and corepack
- Python 3 and uv 0.8.13
- protoc 29.3 with SHA-256 verification
- `protoc-gen-go` v1.36.12 and `protoc-gen-go-grpc` v1.5.1
- GitHub CLI 2.99.0 with SHA-256 verification
- OpenCode CLI 1.18.26
- git, openssh-client, jq, ripgrep, build-essential, shellcheck, sqlite3, rsync, zip, less, procps, and file

All tool versions are fixed through Dockerfile ARG defaults: `DEBIAN_VERSION`, `GO_VERSION`, `NODE_MAJOR`, `PROTOC_VERSION`, `UV_VERSION`, `OPENCODE_VERSION`, and `GH_VERSION`. The architecture-specific amd64/arm64 checksum ARGs are `PROTOC_SHA256_AMD64`, `PROTOC_SHA256_ARM64`, `GH_SHA256_AMD64`, and `GH_SHA256_ARM64`.

The image does not copy the Mouseion checkout, Hermes sessions, credentials, or provider secrets. It uses `/workspace` as the mounted checkout. Hermes runs the container as the host user's uid/gid (`docker_run_as_host_user`, i.e. `--user $(id -u):$(id -g)`), so the agent can write the host-owned checkout and read host-owned credentials in bind mounts. The image is therefore uid-agnostic: the runtime locations (`/home/hermes`, `/workspace`, and the XDG directories) are world-writable (mode `a+rwX`). The in-image `hermes` user (uid/gid 10000) exists only as a non-root fallback when a container is started without a user override.

OpenCode configuration (for example, `config.json`) lives under `$XDG_CONFIG_HOME/opencode` (`/home/hermes/.config/opencode`). Credentials (`auth.json`) live under `$XDG_DATA_HOME/opencode` (`/home/hermes/.local/share/opencode`); the image pre-creates this directory so Hermes can bind-mount the host's `~/.local/share/opencode` over it to supply authentication. Hermes may optionally mount the host's `~/.config/opencode` over the image's configuration directory for configuration.

Credentials must be created on the host with `opencode auth login` — the GitHub Copilot provider accepts **only OAuth tokens**, and Personal Access Tokens are rejected (`Bad Request: checking third-party user token ... Personal Access Tokens are not supported for this endpoint`). The mounted `auth.json` must be readable by the container's runtime uid — under the host-uid override the host's `0600` file already is (owner matches); only a fixed-uid run (no override) would need looser permissions (e.g. `0644` on the host). The image ships without credentials, so authenticated operation can only be smoke-tested at runtime, once a credential is mounted: `opencode run 'Respond with exactly: OPENCODE_SMOKE_OK'`.

## Build and smoke-test locally

From the repository root:

```sh
docker build \
  --file docker/hermes-worker/Dockerfile \
  --tag mouseion-hermes-worker:local \
  docker/hermes-worker
make hermes-worker-smoke HERMES_WORKER_IMAGE=mouseion-hermes-worker:local
```

The smoke test checks that `git`, `gh`, `go`, `node`, `npm`, `corepack`, `python3`, `uv`, `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`, `opencode`, and `rg` are on the runtime user's `PATH` and prints their versions.

## Hermes configuration

After the workflow publishes the private package, use its lowercase GHCR name (replace `OWNER/REPOSITORY`):

```yaml
terminal:
  backend: docker
  cwd: /workspace
  docker_image: ghcr.io/OWNER/REPOSITORY/hermes-worker:latest
  docker_mount_cwd_to_workspace: true
  # Mount the host's authoritative OpenCode config and credentials. Auth
  # (auth.json) lives in ~/.local/share/opencode and must be readable by the
  # container's runtime uid (see the security note). Add :ro if sessions must
  # not write them back.
  docker_volumes:
    - "$HOME/.config/opencode:/home/hermes/.config/opencode"
    - "$HOME/.local/share/opencode:/home/hermes/.local/share/opencode"
  container_persistent: false
  lifetime_seconds: 300
  docker_forward_env: []
```

Hermes runs the container as the host user's uid via `docker_run_as_host_user: true` (appended `--user $(id -u):$(id -g)`), matching the ownership of bind-mounted host files and the checkout. The in-image `hermes` user is only a non-root fallback for containers started without a user override.

`container_persistent: false` gives each session a fresh container; `lifetime_seconds` controls how long an idle session is retained before cleanup. The mounted checkout remains the only project filesystem supplied by the operator. Keep `docker_forward_env` empty unless a task specifically needs a credential: forwarded variables and mounted configuration files are readable by code running in the session. The image contains compilers and network-capable tools, so Docker isolation reduces host exposure but is not a substitute for reviewing the workspace and credentials made available to an agent.

> **Security note:** mounting the host's OpenCode directories exposes its `auth.json` credentials inside the container — readable, and with a read-write mount writable. Only mount directories you trust agents to see; use `:ro` (for example, `"$HOME/.local/share/opencode:/home/hermes/.local/share/opencode:ro"`) if sessions should read credentials but not write them back.

The GHCR package should remain **Private** in its package settings. The publishing workflow uses its job-scoped `GITHUB_TOKEN` with `packages: write`. To pull it locally, authenticate with a GitHub classic personal access token that has only `read:packages` (and repository access):

```sh
printf '%s' "$CR_PAT" | docker login ghcr.io -u GITHUB_USERNAME --password-stdin
docker pull ghcr.io/OWNER/REPOSITORY/hermes-worker:latest
```

Use the full `sha-<commit>` tag emitted by the workflow when a fixed image revision is required. Private-package access can also be granted to other repositories or users from the package settings; do not put those tokens in this repository, the Dockerfile, or image build arguments.
