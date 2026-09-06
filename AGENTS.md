# AGENTS.md

## Project overview

Mouseion is a self-hosted reading environment for learning foreign languages. Go core web server (`cmd/server`) + Python NLP gRPC service (`nlp/`, Stanza) + PostgreSQL, with River as the durable job queue (analysis, enrichment, catalogue sync, prepared-deck translation). `cmd/fixtureserver` is an in-memory server for Playwright browser smoke tests only — it binds no Postgres/NLP/River.

## Sources of truth

- Feature documents: `doc/features/` — product behavior, motivation, scope.
- ADRs: `doc/adr/` — accepted architecture/product decisions; indexed in `doc/product.md`. Add a new ADR to that index when creating one.
- Design docs: `doc/design/` — read `README.md`, `principles.md`, `design-system.md` before substantial frontend/UX work; don't introduce new UI patterns when an established one exists.
- `doc/documentation-governance.md` — repo is authoritative for stable decisions; Obsidian vault is planning material and must not be synced back.
- When docs conflict, executable truth wins; read README + Makefile + CI before assuming anything.

## Commands

Go 1.24, Python 3.11 (pin `3.11`, not newer — Stanza requires `<3.12`), protoc 29.x, templ pinned to `v0.3.977` (newer templ requires Go >= 1.25).

```sh
make setup            # .venv + nlp/requirements-dev.txt + CPU-only torch
make gen              # protobuf -> gen/go, gen/python (source of truth: proto/)
make templ            # templ generate (check in the generated *_templ.go)
make build            # go build ./... + python compileall
make test             # go test ./... + pytest (PYTHONPATH=nlp/src:gen/python)
make test-integration # go test -tags=integration ./internal/...
make lint             # go vet ./... + ruff check nlp/src nlp/tests
make browser-smoke    # Playwright against cmd/fixtureserver
make dev              # go run ./cmd/server (needs Postgres + NLP running)
```

Non-obvious setup:
- Create the Python environment with `uv venv --clear --python 3.11 .venv` before
  running Python tests. Then install the pinned requirements with
  `uv pip install --python .venv/bin/python -r nlp/requirements-dev.txt`,
  `uv pip install --python .venv/bin/python torch --index-url https://download.pytorch.org/whl/cpu`,
  and `uv pip install --python .venv/bin/python -e nlp`. This avoids relying on
  the host `python3` version or Debian's `ensurepip` package.
- `make gen` also needs `protoc` and `protoc-gen-go` on PATH; the Makefile only auto-installs `protoc-gen-go-grpc` v1.5.1 and `grpcio-tools==1.71.2`. Run `make gen` in the venv-configured shell; CI verifies it via `git diff --exit-code`.
- Python commands require `PYTHONPATH=nlp/src:gen/python` and the `.venv` from `make setup`.
- Integration tests use Testcontainers (needs a working Docker daemon) or fall back to `MOUSEION_TEST_DATABASE_URL` (default `postgres://postgres@localhost:5432/mouseion_test`). CI only runs `go test ./...`; verify `-tags=integration` work locally.
- Integration tests use one named, reusable Testcontainers PostgreSQL instance and reset the schema between package processes. The full run can exceed an agent command timeout, so run it in the background and poll its log:
  `mkdir -p .tmp && (make test-integration >.tmp/integration.log 2>&1; printf '%s\n' $? >.tmp/integration.exit) & printf '%s\n' $! >.tmp/integration.pid`
  Check progress with `tail -n 100 .tmp/integration.log`; when the PID exits, read `.tmp/integration.exit` and treat only `0` as passing. Remove the reusable container after the run with `docker rm -f mouseion-test-postgres` when it is no longer needed.
- `make dev` requires a running Postgres (`MOUSEION_DATABASE_URL`) and NLP gRPC (`MOUSEION_NLP_ADDR`, default `localhost:50051`); `MOUSEION_SECRET` is validated at startup.

## Generated artifacts

Regenerate rather than hand-edit. `internal/webapp/*_templ.go` (templ) and `gen/{go,python}` (protobuf) are committed and CI asserts they're current. After regenerating, review the diff to confirm it matches the source change. Migrations are embedded in the server binary via `migrations/embed.go`.

## Schema changes

See ADR 0038. Rules an agent will otherwise get wrong:
- Shipped `migrations/0000NN_*.sql` files are immutable history — never edit, delete, renumber, squash, or consolidate. Corrections are new migrations.
- Settle consequential shape (new tables/columns, constraints, destructive changes, durable state machines) in an accepted issue/feature doc/ADR before writing SQL; low-risk additive fields are exempt only if non-breaking with one clear consumer.
- Keep data-only backfills separate from structural DDL and document ownership, idempotency, retry safety, and rollback/recovery.
- `down` migrations are not assumed safe for destructive production rollback.

## Human-review boundary

`.github/CODEOWNERS` requires human review for: workflows/CI, `migrations/`, `doc/adr/`, `doc/product.md`, `internal/auth/`, `internal/webauth/`, `internal/security/`, `go.mod`/`go.sum`, `nlp/pyproject.toml`, `nlp/requirements*.txt`, Dockerfiles, compose files, `.env*`, and `AGENTS.md`. PRs touching these must not be auto-merged without that review. Do not modify secrets, production, rulesets, or branch protection.

## Repo conventions

- Conventional Commits: `type(scope): imperative summary`.
- Never commit directly to `main`; work on a feature branch and open a PR referencing the issue.
- Local learner accounts: first sign-in on a fresh install creates the account; there is no admin role and no public-registration setting (ADR 0017/0024).
- NLP language availability is discovered from the running service (capabilities), not a web-app allowlist. Deployment-supported languages are `de` and `it`; `make dev` default is `de` only.
- App is meant to be reachable only over the tailnet (plain HTTP, ADR 0009); `MOUSEION_COOKIE_SECURE` only if serving HTTPS.
- Generated decks and known vocabulary are deliberately separate — generating cards never marks vocabulary as known (ADR 0036).

## Agent skills

### Issue tracker

Issues and specs live as GitHub issues in `justin-hayes/mouseion`, managed with the `gh` CLI. See `doc/agents/issue-tracker.md`.

### Triage labels

Issues use the five canonical triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `doc/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` at the repo root plus ADRs under `doc/adr/`. See `doc/agents/domain.md`.
