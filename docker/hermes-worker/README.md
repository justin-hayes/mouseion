# Mouseion Hermes worker image

This is a disposable Docker terminal image for [Hermes Agent](https://hermes-agent.nousresearch.com/docs/user-guide/docker). It is based on `nikolaik/python-nodejs:python3.11-nodejs20` — the same image Hermes uses for its docker backend by default — and adds the tools used by Mouseion agent sessions, running as the base image's non-root `pn` user (uid 1000):

- Go 1.24.6
- protoc 29.3
- `protoc-gen-go` v1.36.12
- `protoc-gen-go-grpc` v1.5.1
- Codex CLI 0.150.1
- Codex profile configs (`fast`, `normal`, `deep`) at `CODEX_HOME=/opt/codex/home`, referencing `OPENCODE_GO_API_KEY` by name (no secrets baked in)
- build-essential, CMake, Git, OpenSSH client, jq, ripgrep, and pkg-config

The image does not copy the Mouseion checkout, Hermes sessions, credentials, or provider secrets. The Docker build context is this directory and only the toolchain stages plus the Codex profile configs are copied into the image. Hermes mounts the selected checkout separately at `/workspace`.

## Build and smoke-test locally

From the repository root:

```sh
docker build \
  --file docker/hermes-worker/Dockerfile \
  --tag mouseion-hermes-worker:local \
  docker/hermes-worker
make hermes-worker-smoke HERMES_WORKER_IMAGE=mouseion-hermes-worker:local
```

The smoke test checks that `go`, `protoc`, both Go protobuf plugins, and `codex` are on the runtime user’s `PATH` and prints their versions.

## Hermes configuration

After the workflow publishes the private package, use its lowercase GHCR name (replace `OWNER/REPOSITORY`):

```yaml
terminal:
  backend: docker
  cwd: /workspace
  docker_image: ghcr.io/OWNER/REPOSITORY/hermes-worker:latest
  docker_mount_cwd_to_workspace: true
  container_persistent: false
  lifetime_seconds: 300
  docker_forward_env: []
```

The image runs as the base image's `pn` user (uid 1000), which matches the default host user on most dev machines — so a host-mounted `/workspace` is writable by default. If your host user's uid differs, set `docker_run_as_host_user: true` so Hermes runs the container as your host uid (appends `--user $(id -u):$(id -g)` to `docker run`).

`container_persistent: false` gives each session a fresh container; `lifetime_seconds` controls how long an idle session is retained before cleanup. The mounted checkout remains the only project filesystem supplied by the operator. Keep `docker_forward_env` empty unless a task specifically needs a credential: forwarded variables and Hermes-mounted credential files are readable by code running in the session. The image contains compilers and network-capable tools, so Docker isolation reduces host exposure but is not a substitute for reviewing the workspace and credentials made available to an agent.

The GHCR package should remain **Private** in its package settings. The publishing workflow uses its job-scoped `GITHUB_TOKEN` with `packages: write`. To pull it locally, authenticate with a GitHub classic personal access token that has only `read:packages` (and repository access):

```sh
printf '%s' "$CR_PAT" | docker login ghcr.io -u GITHUB_USERNAME --password-stdin
docker pull ghcr.io/OWNER/REPOSITORY/hermes-worker:latest
```

Use the full `sha-<commit>` tag emitted by the workflow when a fixed image revision is required. Private-package access can also be granted to other repositories or users from the package settings; do not put those tokens in this repository, the Dockerfile, or image build arguments.
