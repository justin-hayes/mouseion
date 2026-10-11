# Development Guide

How to build, test, and change Mouseion. For running an installation, see the
[operations guide](operations.md).

## Toolchain

- Go 1.27
- Python 3.13, pinned in `nlp/pyproject.toml`, the NLP image, and CI (3.14 needs
  a grpcio upgrade: the pinned grpcio 1.71.2 has no 3.14 wheels)
- Protobuf compiler 29.x for regenerating the shared contract (`protoc-gen-go`
  and `protoc-gen-go-grpc` run from `go.mod` tool directives, with no install step)
- templ `v0.3.1070` (run from the `go.mod` tool directive, with no install step)
- Docker, for Testcontainers-based integration tests
- Node.js 20, only for the Playwright browser suite

Go dependencies are pinned in `go.mod`/`go.sum`; Python development
dependencies are pinned in `nlp/requirements-dev.txt`.

```sh
GO_BIN_DIR="$(go env GOBIN 2>/dev/null)"
[ -n "$GO_BIN_DIR" ] || GO_BIN_DIR="$(go env GOPATH)/bin"
export PATH="$PATH:$GO_BIN_DIR:/usr/local/go/bin"
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- \
  -b "$GO_BIN_DIR" v2.14.0
make setup
make gen
make build
make test
make lint
make check-frontend-css
```

`make setup` creates `.venv` with the pinned development requirements and
CPU-only PyTorch. Python commands need `PYTHONPATH=nlp/src:gen/python`.

Formatting is enforced: `make lint` fails on unformatted Go or Python. Fix it
with `golangci-lint fmt ./...` and `.venv/bin/ruff format nlp/src nlp/tests`.

## Common commands

| Command | Purpose |
| --- | --- |
| `make build` | `go build ./...` plus Python `compileall` |
| `make test` | Go unit tests and pytest |
| `make coverage` | Python and Go coverage reports in `.tmp/coverage/` (the Go integration part needs the `make test-integration` database prerequisites) |
| `make test-integration` | PostgreSQL-backed Go integration tests |
| `make lint` | golangci-lint (including `gofmt` and `goimports` formatting) over the whole module, plus `ruff check` and `ruff format --check` |
| `make browser-smoke` | Playwright suite against the in-memory fixture server |
| `make dev` | Run the web server (needs PostgreSQL and the NLP service) |
| `make gen` | Regenerate protobuf code from `proto/` |
| `make sqlc` | Regenerate `gen/sqlc` from `sqlc/queries/` and the migrations |
| `make templ` | Regenerate `internal/webapp/*_templ.go` |
| `make frontend-css` | Rebuild the committed stylesheet |

## Generated code

Regenerate rather than hand-edit:

- `internal/webapp/*_templ.go` (templ, from `internal/webapp/*.templ`),
  `gen/go` and `gen/python` (protobuf, from `proto/`), and `gen/sqlc` (sqlc
  v1.31.1) are committed; CI regenerates them and fails on any difference.
- Run `make templ` after changing a `.templ` file and commit both. Go-compiling
  `make` targets also regenerate templ output when the pinned templ is
  installed, and otherwise build the committed files, so a fresh checkout
  builds with plain `go build` and `go test`.
- Never hand-resolve a conflict in generated code. Resolve the source, then
  regenerate and stage the output. Generated templ output records source line
  and column positions, so it often conflicts even when the `.templ` edits
  merge cleanly.

### Code scanning

CodeQL runs with GitHub's default setup. Its Go autobuild compiles the
committed templ output, which is one reason that output is committed: without
it, CodeQL cannot type-check `internal/webapp`. Code behind the `integration`
build tag is not analysed.

## Tests

- **Unit tests** — `go test ./...` and `make test`. Results use Go's normal
  test cache.
- **Integration tests** — `make test-integration` runs every package with the
  `integration` build tag and `-count=1`, so results are never cached. They use
  one named, reusable Testcontainers PostgreSQL instance; a template database is
  migrated once per migration set and every test gets its own clone, so tests
  are isolated and packages run in parallel. Without Docker, set
  `MOUSEION_TEST_DATABASE_URL` to a PostgreSQL role with `CREATEDB`. Remove the
  reusable container with `docker rm -f mouseion-test-postgres`.
  `make test-integration-shared` runs the shared-database runner.
- **Targeted runs** — `make test-go PACKAGES='./internal/webapp ./internal/foo'`
  or `make test-go-integration PACKAGES='./internal/persistence'`. Both put Go
  build temporary files in a per-run directory under `.tmp/go-test`. If a
  process is killed and leaves a directory behind, `make go-test-clean` removes
  only this user's `run.*` directories whose recorded process is no longer
  running.
- **Browser acceptance** — `make browser-smoke` runs Playwright against
  `cmd/fixtureserver`, a deterministic in-memory server with no PostgreSQL,
  River, or NLP ([ADR 0033](adr/0033-browser-acceptance-harness.md)). Chromium
  runs the full suite across desktop and compact viewports in light and dark
  schemes; WebKit runs a focused native-behavior journey. See
  [`e2e/README.md`](../e2e/README.md).
- **Python** — pytest under `nlp/tests` covers the NLP producer, gRPC server,
  model provisioning, dictionary-index derivation, and the precision
  measurement script.

### Test coverage

`make coverage` writes the NLP service's Python coverage as a Cobertura XML
report to `.tmp/coverage/python.xml`, measured over `nlp/src` with pytest-cov
(pinned in `nlp/requirements-dev.txt`). Its file paths are relative to the
repository root, so SonarQube Cloud can match them to sources. It then runs the
Go unit tests and the Go integration tests, each with a coverage profile, and
writes the filtered profiles to `.tmp/coverage/go-unit.out` and
`.tmp/coverage/go-integration.out`. `.tmp/` is gitignored, so the reports are
never committed. The target fails before running any test when `.venv` or pytest
is missing. `tools/go-coverage.sh`
removes generated code (`gen/` and `*_templ.go`) from each Go profile with
`tools/coverage-filter.sh`. `tools/coverage-filter_test.sh` checks that filter.

The integration profile uses `-coverpkg=./...`, so coverage is credited to every
module package the integration tests execute, such as the PostgreSQL store, not
only to the package under test. `make coverage` needs the same database
prerequisites as `make test-integration`: a running Docker daemon for
Testcontainers, or `MOUSEION_TEST_DATABASE_URL` pointing at a PostgreSQL role
with `CREATEDB`.

Test coverage here means test line coverage: the share of statements that the
tests execute. It is not vocabulary coverage, the learner-facing measure in
[#1732](https://github.com/justin-hayes/mouseion/issues/1732). The two are
unrelated, and a figure from one never describes the other.

### Static analysis (SonarQube Cloud)

`.github/workflows/sonarqube.yml` analyses the repository in SonarQube Cloud
(organisation `justin-hayes`, project `justin-hayes_mouseion`) on every push to
`main` and on every pull request. It runs on a GitHub-hosted runner, which
provides Docker for the integration tests' Testcontainers PostgreSQL. It runs the
filter behavior test, sets up the Python environment the same way `ci.yml` does,
runs `make coverage` (Python, Go unit, and Go integration), and then runs the
official scan action. The scan's settings live in
`sonar-project.properties` at the repository root. Excluded paths are
generated code, vendored static assets, the generated stylesheet, the
immutable baseline migration, and the deliberate EPUB and card-export test
fixtures. Intended behaviour that Sonar flags is recorded there as
issue-ignore criteria, each scoped to one rule and one file, with its reason in
a comment. Python test files under `nlp/tests` are classified as tests, not
sources.

The scan reads the Go profiles from `sonar.go.coverage.reportPaths` and the
Python report from `sonar.python.coverage.reportPaths`, so coverage for both
languages reaches SonarQube Cloud as part of the same analysis.

Fork pull requests cannot read repository secrets, so `SONAR_TOKEN` is
unavailable there. The scan step is skipped for them, and the coverage steps
still run, so the job succeeds without an analysis. Those pull requests get no
SonarQube Cloud analysis and no decoration.

## Regenerating screenshots

`make screenshot-my-books` regenerates the README screenshots in `doc/images/`:
`my-books.png`, `my-books-dark.png`, `reading.png`, `concordance.png`, and
`sentence-study.png`. They come from a real run of Mouseion's analysis on pinned
public-domain German novels from Project Gutenberg, on an isolated Compose stack
(project `mouseion-screenshot`, web port bound to `127.0.0.1`). Nothing is
written unless every assertion passes. The tool is run by hand. It is **not part
of CI**: no CI job runs it, and changes under `e2e/screenshot/**` and `doc/**`
skip the build, test, and browser checks (see the CI change classes above).

Prerequisites:

- Docker with Compose v2.24.4 or newer, and a running daemon that your user can
  reach.
- Node.js 20 or newer with npm. The target installs Playwright's Chromium.
- Network access to `www.gutenberg.org` on the first run, to fetch the pinned
  sources. Later runs use the cache in `.tmp/screenshot/sources`. The first run
  also downloads the Stanza and Hugging Face models into Docker volumes.

Run it from the repository root:

```sh
make screenshot-my-books
```

The run prints the Known-vocabulary cut-off and the coverage it measured. Record
that output in any pull request that changes the committed images.

Duration, measured on the maintainer's workstation (16 cores, 31 GB RAM):

- First run, after `make screenshot-my-books-clean`, with the sources cache and
  built images removed (Docker's build cache kept): about 15 minutes. It
  re-downloads the pinned sources and the models.
- Repeat run, with the models and sources cached: about 15 minutes. The browser
  workflow itself took 13.8 minutes in both runs.

The Playwright test budget is 75 minutes. A run on unchanged code reproduces the
committed PNGs byte for byte.

Disk usage, measured on the same runs:

- Stanza model volume about 330 MB and Hugging Face model volume about 330 MB.
  Both are kept between runs.
- Database volume about 860 MB. It is recreated on the next run.
- Built images: the `nlp` image is about 1.7 GB and the `web` image about 70 MB.
- Scratch files under `.tmp/screenshot/`, about 8 MB, including the cached
  sources.

Clean-up:

- `make screenshot-my-books-clean` removes the project's containers and volumes,
  including the model caches, so the next run downloads them again. It does not
  remove built images or `.tmp/screenshot/`.
- Built images can be removed with `docker image rm` for the
  `mouseion-screenshot-*` images, and `rm -rf .tmp/screenshot` drops the cached
  sources.

## Frontend

The web application is server-rendered with [templ](https://templ.guide/) and
enhanced with htmx. All pages load one committed, compiled stylesheet at
`/static/app.css`. Tailwind scans only authored `internal/webapp/*.templ`
files, the explicit Go class mappings, and the owned enhancement script;
generated templates, tests, vendor assets, and temporary files are excluded.
After changing those inputs or the CSS sources under `internal/webapp/styles/`,
run `make frontend-css` and verify with `make check-frontend-css`. Go builds and
the fixture server use the committed output and need neither Node.js nor a CSS
compiler.

Concordance occurrences are rendered once by the server as native disclosures
with ordinary paging and study links
([ADR 0083](adr/0083-concordance-server-rendering-and-htmx-4.md)). Browser tests
exercise both enhanced requests and JavaScript-disabled navigation.

Read [`design/README.md`](design/README.md), the
[design principles](design/principles.md), and the
[design system](design/design-system.md) before substantial UI work.

## Schema changes

`migrations/000001_initialize.{up,down}.sql` is the immutable current-state
baseline; new migrations resume at `000002` and are immutable once shipped
([ADR 0070](adr/0070-migration-and-documentation-reboot.md)). Settle
consequential schema shape in an accepted feature document or ADR before
writing SQL, and keep data-only backfills separate from structural DDL
([ADR 0038](adr/0038-schema-change-governance.md)).

## Continuous integration

CI runs on ephemeral GitHub-hosted `ubuntu-24.04` runners, so pull requests
from forks never execute on maintainer infrastructure. The build, lint,
integration, and browser-smoke jobs run in parallel; browser smoke is split
across four Playwright shards behind a single `Browser smoke` check. Shards run
the suite in a digest-pinned Playwright image that already contains the browsers
and their system libraries, so they do not install APT packages or download
browsers ([e2e/README.md](../e2e/README.md) describes the pin and its coupling
to the locked `@playwright/test` version). Each job
starts from a clean machine and restores Go, uv, npm, and golangci-lint
dependencies from GitHub Actions caches keyed on the corresponding lock files.
A `changes` job classifies each pull request once and gates the other jobs on
the result, so a job a change cannot affect is skipped before any runner,
container, or browser is provisioned. Integration tests start PostgreSQL
through Testcontainers on the runner's Docker daemon.

### CI change classes

`tools/ci-changes.sh` maps each changed path to the checks it affects; its
`case` table is the only definition. The first matching rule wins, so order
matters. A path no rule matches runs every check, and manual dispatch does too.
Build tooling (`Makefile`, `tools/**`, `.github/workflows/**`) also runs every
check. "Build and test" runs the Go steps for `go`, the Python steps for
`python`, and the regeneration checks for `generated`; `frontend` adds the
stylesheet check.

| Change | Build and test | Go lint and integration | Browser smoke |
| --- | --- | --- | --- |
| Documentation (`doc/**`, `*.md`, `CITATION.cff`, `LICENSE`, `SECURITY.md`) | skipped | skipped | skipped |
| Screenshot project (`e2e/screenshot/**`) | skipped | skipped | skipped |
| NLP code (`nlp/**`) | Python | skipped | skipped |
| NLP expectations read by Go tests (`nlp/tests/testdata/**`, `nlp/testdata/**`) | Go and Python | run | skipped |
| Shared npm dependencies and Playwright configuration (`e2e/package*.json`, `e2e/playwright.config.ts`, `e2e/support/**`) | skipped | skipped | run |
| Browser tests (`e2e/tests/**`) | skipped | skipped | run |
| Stylesheet source (`internal/webapp/styles/**`) | stylesheet check | skipped | skipped |
| Templates (`*.templ`) | Go, stylesheet, and regeneration | run | run |
| Stylesheet classes and embedded assets (`internal/webapp/components.go`, `internal/webapp/static/**`) | Go and stylesheet | run | run |
| Fixture server and packages it compiles (`cmd/fixtureserver/**`, `internal/**` non-test code) | Go | run | run |
| Other Go code and Go test inputs (`cmd/**`, `*_test.go`, `testdata/**`, test helpers) | Go | run | skipped |
| Embedded Go data (`internal/canonicalization/german_post1996.json`, `internal/cardexport/templates/**`) | Go | run | run |
| Protobuf and sqlc inputs (`proto/**`, `sqlc/**`, `sqlc.yaml`) | Go and regeneration (Python for `proto/**`) | run | skipped |
| Committed generated Go, sqlc, and templ (`gen/go/**`, `gen/sqlc/**`, `internal/webapp/*_templ.go`) | Go and regeneration | run | run |
| Committed generated Python (`gen/python/**`) | Python and regeneration | skipped | skipped |
| Migrations (`migrations/**`) | Go and regeneration | run | run |
| Go dependencies (`go.mod`, `go.sum`) | Go and regeneration | run | run |
| Python dependencies (`nlp/requirements*.txt`, `nlp/pyproject.toml`) | Python and regeneration | skipped | skipped |
| Build tooling, workflow, unmatched paths, manual dispatch | every check | run | run |

Browser smoke is a single required check. It passes when its four shards
succeed, or when they are skipped because the classification did not require
browser checks. A failed, cancelled, or unexpectedly skipped shard fails it,
and so does a failed change detection. `tools/ci-gate.sh` holds that rule.
Gated jobs that a change does not require are skipped as a whole, which GitHub
reports as passing for required checks; a failed change detection makes the
gated jobs run and fail instead.

When a build or embedding dependency changes, update the rules and the
expectations together:

- New `//go:embed` input: add its path to the matching rule in
  `tools/ci-changes.sh`. Find them with
  `grep -rn 'go:embed' --include=*.go internal cmd migrations`.
- New runtime file read by a Go test from outside its package (for example
  `../../nlp/testdata/`): add it to the Go rule. Find them with
  `grep -rn 'ReadFile("\.\./' --include=*_test.go internal cmd`.
- Browser inputs: the fixture server's compiled packages are
  `go list -deps ./cmd/fixtureserver`. Packages under `internal/` are covered
  by the browser rule except test helpers, and a new fixture dependency outside
  `internal/` or `cmd/fixtureserver/` needs its own rule.
- Add a case to `tools/ci-changes_test.sh` for each new class, with the job
  selection it must produce. The test also checks that each gated job's `if:`
  tests the expected class and that the change-detection guards are present.
  The `changes` job runs it before it classifies a pull request; run it locally
  with `tools/ci-changes_test.sh`.

### Browser fixture contract

The `Browser fixture` job builds
`cmd/fixtureserver` once per run attempt, only when browser inputs change or the
run is a manual dispatch. It uploads the binary, a `commit` file holding
`GITHUB_SHA`, and a `platform` file holding `uname -sm`, as the artifact
`browser-fixture-<sha>-<run id>-<run attempt>`. The artifact name is exposed as
a job output. The four `Browser smoke shard` jobs depend only on this job, not on
the build, lint, Python, or integration jobs. Each shard downloads that artifact
before any browser setup, verifies the commit and platform, restores the
executable bit, and runs Playwright with `MOUSEION_FIXTURE_BIN`. Playwright
still starts an independent fixture process per worker and spec file.

Re-running only failed jobs reuses the artifact from the earlier attempt, which
the job output still names; it stays bound to the same commit. Failure of the
fixture build skips the shards, and the `Browser smoke` check fails for that
skip. Runs whose change classification does not require browser checks skip the
fixture job, the upload and download, and the shards; the required check still
reports success. Local `make browser-smoke` is unchanged and still runs
`go run` through Playwright.

## Dependency updates

Dependabot version updates are configured in
[`.github/dependabot.yml`](../.github/dependabot.yml). Each ecosystem entry
follows the same conventions, so later ecosystems copy the pattern rather than
inventing their own.

- **What it updates:** GitHub Actions (`github-actions`, `/`), Go modules
  (`gomod`, `/`), the root `Dockerfile` image (`docker`, `/`), the root
  `compose.yaml` images (`docker-compose`, `/`), Python NLP dependencies
  (`pip`, `/nlp`), and the browser acceptance npm packages (`npm`, `/e2e`).
- **Schedule:** weekly, on Monday at 06:00 Berlin time.
- **Grouping:** minor and patch updates share one pull request per ecosystem.
  Major updates are not grouped, so each breaking change gets its own review.
- **Open pull request cap:** at most five open version-update pull requests per
  ecosystem.
- **Commit messages:** Conventional Commit prefixes, `ci(deps)` for GitHub
  Actions and `build(deps)` for application ecosystems.
- **Pinning:** GitHub Actions stay pinned to a full commit SHA with the version
  as a trailing comment. Dependabot updates both together. A reason for keeping
  an action on an old version goes in a comment line above its `uses:` line,
  because Dependabot rewrites the trailing comment.
- **Review:** Dependabot pull requests are not auto-merged. The configuration,
  workflows, and dependency manifests are in `.github/CODEOWNERS` and need human
  review.

### Security alerts

Known vulnerabilities in dependencies appear under the repository's
**Security** tab, in Dependabot alerts. When Dependabot security updates are
enabled in the repository settings, Dependabot opens a pull request for each
alert it can fix. Pull requests also run dependency review
(`.github/workflows/dependency-review.yml`), which flags newly introduced
vulnerable dependencies.

### Ignore rules and security updates

GitHub documents that an `ignore` rule in `dependabot.yml` applies to both
version updates and security updates for the named dependency
([Controlling which dependencies are updated by Dependabot](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/manage-your-dependency-security/controlling-dependencies-updated)).
Two consequences follow:

- Excluding a dependency also stops its security pull requests. An excluded
  dependency has to be watched through its Dependabot alerts by hand.
- `update-types` in an `allow` rule restricts only version updates. GitHub
  documents that security updates are created regardless, so `update-types`
  cannot hold back a security fix.

### Exclusions

Each exclusion needs an `ignore` rule in `dependabot.yml`, a reason, and a
tracking issue. Each rule carries its reason as a comment in `dependabot.yml`.

| Dependency | Ecosystem | Reason | Lifted by | Tracking issue |
| --- | --- | --- | --- | --- |
| `github.com/a-h/templ` | `gomod` | The module must match the templ CLI pin (`TEMPL_VERSION`), which generates the committed views. Newer templ requires Go 1.25 or later. | Manual bump, below. | #1742 |
| `google.golang.org/protobuf` | `gomod` | Must equal the `protoc-gen-go` tool version in `go.mod`, so the runtime and the generator that verify `gen/` move together. | Manual bump, below. | #1742 |
| `golang` (tags `>= 1.28`) | `docker` | The Go toolchain is set by the `go` directive in `go.mod`, which CI reads. An image ahead of it would drift from the build. | Manual Go bump, below. | #1742 |
| `grpcio`, `grpcio-tools`, `protobuf` | `pip` | `grpcio-tools` generates the committed Python protobuf code that CI verifies. grpcio is held at 1.71.2 for the Python 3.13 pin; Python 3.14 needs a grpcio upgrade first. | Manual Python protobuf bump, below. | #1743 |
| `@playwright/test` | `npm` (`/e2e`) | Must match the digest-pinned Playwright image in `ci.yml`, which Dependabot cannot update. CI fails when they differ. | Manual Playwright bump: [`e2e/README.md`](../e2e/README.md), "Dependency and update policy". | #1743 |

Excluding a dependency also stops its security pull requests, so watch the
Dependabot alerts for each excluded dependency and bump it by hand when one
appears.

#### Go directive raises

Dependabot cannot express "updates that raise the `go` directive" as an ignore
rule. A `gomod` pull request whose diff changes the `go` line in `go.mod` is
therefore closed without merging, unless the maintainer is doing the Go bump
below. The `golang` ignore rule covers the Docker image side only.

### Manual bumps

These pins are not in files Dependabot reads, or are excluded above. Bump each
one by hand, then regenerate and verify as listed.

- **templ** (`github.com/a-h/templ`, `TEMPL_VERSION` in the `Makefile`):
  1. Check the release's minimum Go version against the `go` directive.
  2. `go get github.com/a-h/templ@vX.Y.Z`, then update `TEMPL_VERSION` to the
     same version.
  3. `make templ && make build test lint`, then commit
     the regenerated `internal/webapp/*_templ.go`.
- **protoc-gen-go and the protobuf runtime** (`google.golang.org/protobuf` in
  `go.mod`, and the `protoc-gen-go` tool directive in `go.mod`):
  1. `go get google.golang.org/protobuf@vX.Y.Z` and
     `go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@vX.Y.Z`, so the
     tool and the runtime match.
  2. `make gen`, then review `git diff gen/` against the source change. The
     generated headers record the generator version.
  3. `make build test`.
- **protoc-gen-go-grpc** (the `protoc-gen-go-grpc` tool directive in `go.mod`,
  currently `v1.5.1`): run `go get -tool google.golang.org/grpc/cmd/protoc-gen-go-grpc@vX.Y.Z`,
  then run `make gen` and review the `gen/go` diff. Its runtime, `google.golang.org/grpc`,
  is a normal Dependabot-managed module.
- **Go toolchain** (the `go` directive in `go.mod`, the `golang:` tag in the
  `Dockerfile`, and the `golang` ignore rule's `>= 1.28` bound in
  `dependabot.yml`): raise all three together. CI reads `go.mod`, so it follows
  without a separate edit. Move the `>= 1.28` bound up with the image, so the
  `docker` ecosystem can propose the next one.
- **sqlc** (`SQLC_VERSION` in the `Makefile`, `sqlc-version` in the
  `sqlc-dev/setup-sqlc` step of `ci.yml`): change both, then run `make sqlc` and
  review the `gen/sqlc` diff against the query change.
- **golangci-lint** (`GOLANGCI_LINT_VERSION` in the `Makefile`, `version` in the
  `golangci/golangci-lint-action` step of `ci.yml`): change both, then run
  `make lint`.
- **grpcio, grpcio-tools, protobuf** (Python; `grpcio==` and `protobuf==` in
  `nlp/pyproject.toml`, all three pins in `nlp/requirements-dev.txt`, and the
  `grpcio-tools==` check in the `Makefile` `gen` target):
  1. Change all of those to the target versions together, and confirm the target
     has wheels for the Python version the project pins (3.13 today).
  2. `make setup`, then `make gen`, and review `git diff gen/` against the
     change. `gen/python` is committed and CI checks it.
  3. `make build test lint`.
- **@playwright/test** (`e2e/package.json`, `e2e/package-lock.json`, and the
  `PLAYWRIGHT_IMAGE` tag and digest in `ci.yml`): do not bump it alone. Follow
  [`e2e/README.md`](../e2e/README.md), "Dependency and update policy", which
  moves the package, lockfile, and image together.

## Conventions

- [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): imperative summary`.
- Work on a feature branch and open a pull request that references its issue.
- Use the domain vocabulary in [`CONTEXT.md`](../CONTEXT.md).
- Record consequential decisions as ADRs and add them to the index in
  [`product.md`](product.md).
- Paths listed in `.github/CODEOWNERS` (workflows, migrations, ADRs, auth and
  security code, dependency manifests, containers) require human review; see
  the [autonomous change policy](autonomous-change-policy.md).
