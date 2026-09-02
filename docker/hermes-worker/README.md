# Mouseion Hermes worker image

This is a disposable Docker terminal image for [Hermes Agent](https://hermes-agent.nousresearch.com/docs/user-guide/docker). It is based on `debian:bookworm-slim` and provides the tools used by Mouseion agent sessions. The image runs as a fixed in-image `hermes` user (uid/gid 10000).

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

The image does not copy the Mouseion checkout, Hermes sessions, credentials, or provider secrets. It uses `/workspace` as the mounted checkout. The fixed runtime user is `hermes` (uid/gid 10000), and Hermes runs the container as that user.

OpenCode configuration (for example, `config.json`) lives under `$XDG_CONFIG_HOME/opencode` (`/home/hermes/.config/opencode`). Credentials (`auth.json`) live under `$XDG_DATA_HOME/opencode` (`/home/hermes/.local/share/opencode`); the image pre-creates this directory, owned by the runtime user, so Hermes can bind-mount the host's `~/.local/share/opencode` over it to supply authentication. Hermes may optionally mount the host's `~/.config/opencode` over the image's configuration directory for configuration.

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
  # Mount the host's authoritative OpenCode config + auth over the image's
  # /home/hermes/.config/opencode. Add :ro if sessions must not write it.
  docker_volumes:
    - "$HOME/.config/opencode:/home/hermes/.config/opencode"
  container_persistent: false
  lifetime_seconds: 300
  docker_forward_env: []
```

The image uses a fixed in-image user (uid/gid 10000), and Hermes runs it as that user. The operator's Hermes configuration was updated to match; host-uid run flags were removed.

`container_persistent: false` gives each session a fresh container; `lifetime_seconds` controls how long an idle session is retained before cleanup. The mounted checkout remains the only project filesystem supplied by the operator. Keep `docker_forward_env` empty unless a task specifically needs a credential: forwarded variables and mounted configuration files are readable by code running in the session. The image contains compilers and network-capable tools, so Docker isolation reduces host exposure but is not a substitute for reviewing the workspace and credentials made available to an agent.

> **Security note:** mounting `~/.config/opencode` makes the host's OpenCode credentials (`auth.json`) readable — and, with a read-write mount, writable — inside the container. Only mount a configuration directory you trust agents to see; use `:ro` (for example, `"$HOME/.config/opencode:/home/hermes/.config/opencode:ro"`) if sessions should read configuration but not write it back.

The GHCR package should remain **Private** in its package settings. The publishing workflow uses its job-scoped `GITHUB_TOKEN` with `packages: write`. To pull it locally, authenticate with a GitHub classic personal access token that has only `read:packages` (and repository access):

```sh
printf '%s' "$CR_PAT" | docker login ghcr.io -u GITHUB_USERNAME --password-stdin
docker pull ghcr.io/OWNER/REPOSITORY/hermes-worker:latest
```

Use the full `sha-<commit>` tag emitted by the workflow when a fixed image revision is required. Private-package access can also be granted to other repositories or users from the package settings; do not put those tokens in this repository, the Dockerfile, or image build arguments.
