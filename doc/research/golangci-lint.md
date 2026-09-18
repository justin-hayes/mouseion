# Adopting golangci-lint

**Research date:** 2026-09-17

**Scope:** Research and documentation only. This report does not change the
repository configuration, Makefile, or workflows.

## Executive Recommendation

Adopt the v2 line and pin `golangci-lint` to the exact binary release
`v2.13.2`. At the research date, the official release API identifies
`v2.13.2` as a stable, non-prerelease release published on 2026-08-27
([release API](https://api.github.com/repos/golangci/golangci-lint/releases/tags/v2.13.2),
[release page](https://github.com/golangci/golangci-lint/releases/tag/v2.13.2)).

Use the v2 configuration schema with `version: "2"`, and select linters
explicitly rather than using `default: all`. The official CI guidance warns
that `default: all` can cause builds to start failing when a new linter is
added or an upstream linter changes ([CI installation guidance](https://golangci-lint.run/docs/welcome/install/ci/)).

For local development, install the official binary at `v2.13.2`; do not make
the project compile golangci-lint as a project dependency. The official local
installation documentation recommends binary installation and warns that
source installation depends on the local Go version and can produce an
untested result ([local installation](https://golangci-lint.run/docs/welcome/install/local/)).

For GitHub Actions, use the official action `v9.0.0`, with
`version: v2.13.2`. The action's v9 release uses the Node 24 runtime and its
metadata declares binary installation as the default
([v9 release](https://github.com/golangci/golangci-lint-action/releases/tag/v9.0.0),
[v9 action metadata](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/action.yml)).
For supply-chain hardening, reference the action by its full commit SHA with a
`# v9.0.0` comment; GitHub documents a full-length SHA as the immutable action
reference ([GitHub secure-use guidance](https://docs.github.com/en/actions/reference/security/secure-use#using-third-party-actions)).

Run golangci-lint in its own CI job, after `setup-go`, and let the action's
analysis cache complement `actions/setup-go`'s Go module/build cache. The
official action recommends a separate job and documents both cache layers
([official action README, v9.0.0](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md)).

Start with the clean, focused configuration in this report and run it over the
entire tree, including files selected by the `integration` build tag. Do not
start with the broader 23-linter candidate set: an actual v2.13.2 baseline run
found 467 issues in the normal build and 708 with integration-tagged files.
Add stricter linters only after reviewing and clearing each linter's baseline.

Replace the separate `go vet ./...` invocation once golangci-lint is adopted.
The official linter description says `govet` is roughly the same as `go vet`
and uses its passes ([`govet` documentation](https://golangci-lint.run/docs/linters/#govet)); in this repository both commands completed with zero findings.

## Repository Observations

These are observations of the repository, not claims about golangci-lint:

- [`go.mod`](../../go.mod#L1-L20) declares module `github.com/justin-hayes/mouseion`, Go `1.24.0`, and a mixed application dependency set including gRPC, pgx, River, SQLite, and Testcontainers.
- [`Makefile`](../../Makefile#L1-L8) has existing tool-version variables for generated tooling, but no golangci-lint variable.
- [`Makefile`](../../Makefile#L67-L69) currently runs `go vet ./...` and Ruff for Python linting; the `lint` target is therefore already the natural cross-language entry point.
- [`ci.yml`](../../.github/workflows/ci.yml#L74-L100) obtains Go from `go.mod` through `actions/setup-go@v5`, enables its cache, and runs separate build, test, and `go vet` steps.
- [`ci.yml`](../../.github/workflows/ci.yml#L28-L68) uses `dorny/paths-filter@v3` and has a Go filter for Go sources, module files, protobuf, SQLC, migrations, the Makefile, and the workflow, but it does not currently include `.golangci.yml`.
- Generated protobuf files under [`gen/go`](../../gen/go/) begin with the strict generated-code marker, for example [`normalized_corpus.pb.go`](../../gen/go/mouseion/v1/normalized_corpus.pb.go#L1-L5).
- Generated SQLC files under [`gen/sqlc`](../../gen/sqlc/) use the same marker, for example [`models.go`](../../gen/sqlc/models.go#L1-L4).
- Generated templ output also uses the marker, for example [`components_templ.go`](../../internal/webapp/components_templ.go#L1-L4), and already contains a file-level generated-code lint suppression at [`components_templ.go`](../../internal/webapp/components_templ.go#L6-L6).
- The repository is mixed Go/Python, but golangci-lint only analyzes Go; the existing Ruff commands should remain the Python lint path. The separate language commands are visible in [`Makefile`](../../Makefile#L67-L69) and [`ci.yml`](../../.github/workflows/ci.yml#L171-L175).

## Measured Baseline

The recommendations below were tested with the official v2.13.2 Linux binary,
not inferred only from the dependency list. The candidate configuration was
schema-validated before each run, and the final focused configuration also
passed while loading the repository with Go 1.24.0 and the `integration` tag.

| Linter | Normal build | With `integration` tag |
| --- | ---: | ---: |
| `testifylint` | 231 | 359 |
| `errcheck` | 90 | 182 |
| `contextcheck` | 34 | 35 |
| `staticcheck` | 29 | 31 |
| `modernize` | 21 | 21 |
| `errorlint` | 16 | 16 |
| `unconvert` | 14 | 14 |
| `unused` | 14 | 22 |
| `ineffassign` | 7 | 10 |
| `intrange` | 4 | 9 |
| `nilerr` | 3 | 3 |
| `predeclared` | 2 | 4 |
| `rowserrcheck` | 1 | 1 |
| `wastedassign` | 1 | 1 |
| **Total** | **467** | **708** |

The proposed initial linters below produced zero findings both with and without
the `integration` tag, including the Go 1.24.0 verification run. Generated-file
suppression also worked as intended: no findings were reported from `gen/go`,
`gen/sqlc`, or `*_templ.go`.

## Issue 1017 Rollout

The issue 1017 baseline was recounted against the post-1016 `main` tree with
golangci-lint v2.13.2 and the repository's `integration` build tag. The
candidate findings were:

| Linter | Findings before cleanup | Findings after cleanup |
| --- | ---: | ---: |
| `modernize` | 28 | 0 |
| `intrange` | 9 | 0 |
| `unconvert` | 12 | 0 |
| `predeclared` | 5 | 0 |
| `perfsprint` | 44 | 0 |
| `usestdlibvars` | 2 | 0 |
| `errname` | 1 | 0 |

All seven candidates are now required gates. The changes are limited to
mechanical Go 1.24 modernization, equivalent standard-library helpers,
unnecessary conversion removal, names that shadow predeclared identifiers,
standard-library constants, and the conventional `Error` suffix for an error
type. No generated file was edited. The `unconvert` changes remove only casts
that the compiler already treats as assignable; casts at actual type or
serialization boundaries remain in place.

The baseline also exposed project-specific policy questions:

- Most `errcheck` findings are ignored `Rollback` and `Close` results. These
  need a deliberate cleanup/exclusion policy rather than bulk suppression.
- Most `contextcheck` findings follow templ-generated component call chains.
  Do not enable it until those false positives can be configured narrowly.
- `staticcheck` combines high-value `SA` checks with style and simplification
  checks. Its current findings include one deprecated `strings.Title` use and
  one empty branch, but also many capitalization and rewrite suggestions.
- `testifylint`, `modernize`, `intrange`, `predeclared`, and `unconvert` are
  predominantly test-style or modernization policy. They should not become
  required merely because golangci-lint makes them available.
- `nilerr` (3), `rowserrcheck` (1), and `wastedassign` (1) are small,
  high-signal cleanup batches and are the best first additions after adoption.

## Version And Go Compatibility

### Sourced facts

- The v2.13.2 source `go.mod` declares `go 1.26.0` and identifies the module as `github.com/golangci/golangci-lint/v2` ([tagged source](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/go.mod)).
- The official FAQ says golangci-lint supports Go versions lower than or equal to the Go version used to compile it, while also stating that its supported policy follows the two latest Go minor versions ([tagged FAQ](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/welcome/faq.md#which-go-versions-are-supported)).
- The official installer provides an exact `v2.13.2` binary command and separately warns that `go install` compiles locally ([local installation](https://golangci-lint.run/docs/welcome/install/local/)).

### Project-specific recommendation

Use the prebuilt `v2.13.2` binary with this repository's Go 1.24 module. The
binary's build Go version is not a recommendation to change this module's Go
directive, and binary installation avoids trying to compile v2.13.2 with the
repository's older Go toolchain. Because the official policy statement refers
to the two latest Go minors, validate `v2.13.2` against this Go 1.24 codebase
in the verification phase rather than treating compatibility as guaranteed by
the module directive alone.

Pin the complete patch version, not merely `v2.13` or `latest`. The action
accepts a minor version and resolves it to a patch version, but an exact patch
pin prevents a later patch release from changing lint results without a
repository change ([action version input](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#version)).

## Installation Model

### Local

The official binary installer supports an exact version and installation into
`$(go env GOPATH)/bin` ([local binary installation](https://golangci-lint.run/docs/welcome/install/local/#binaries)):

```sh
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- \
  -b "$(go env GOPATH)/bin" v2.13.2
export PATH="$(go env GOPATH)/bin:$PATH"
golangci-lint version
```

The current README's PATH example does not fall back to `GOPATH/bin` when
`GOBIN` is unset, because `go env GOBIN` succeeds with empty output. Correct
that example during implementation so a default Go installation can find the
installed binary.

The proposed Makefile should use a configurable executable rather than
silently downloading software, and should reject version drift:

```make
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.13.2
```

`GOLANGCI_LINT_VERSION` documents the CI/local contract; the executable itself
can be supplied through `PATH` or `GOLANGCI_LINT=/path/to/binary`. A future
implementation should check `golangci-lint version` before running so local
invocations cannot silently use another version. The Makefile and action inputs
are duplicate pins and must be updated together in a reviewed change.
This avoids changing `go.mod` and respects the upstream recommendation not to
use `go install` as the normal installation mechanism
([installation warning](https://golangci-lint.run/docs/welcome/install/local/#install-from-sources)).

### CI

Use the official GitHub Action with an explicit Go setup step. The action v4+
compatibility notes require an explicit `setup-go` step, and v5+ delegates Go
module/build caching to `actions/setup-go` ([action compatibility](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#compatibility)).

The action's analysis cache is enabled by default. Its documented cache key
contains the runner OS, working directory, a periodic invalidation number, and
the `go.mod` hash; the default invalidation interval is seven days
([action cache options](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#cache),
[cache internals](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#caching-internals)).
Do not add the removed `skip-pkg-cache` or `skip-build-cache` options; the
action documents that Go caching is handled by `setup-go` in v5 and later
([compatibility notes](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#compatibility)).

The action version is a separate pin from the linter version:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
    cache: true
- name: Go lint
  uses: golangci/golangci-lint-action@0a35821d5c230e903fcfe077583637dea1b27b47 # v9.0.0
  with:
    version: v2.13.2
```

Keep the repository's existing `setup-go@v5` during adoption: the
golangci-lint action requires an explicit setup step but does not require v6.
The current official examples use v6, whose release moves to Node 24 and
requires runner `v2.327.1` or later ([setup-go v6 README](https://raw.githubusercontent.com/actions/setup-go/v6.0.0/README.md),
[setup-go v6 release](https://github.com/actions/setup-go/releases/tag/v6.0.0)).
That upgrade is independent and should not be bundled into this change.

## Proposed Configuration

This is the recommended initial `.golangci.yml`; it is not being added by this
research task. The schema and option names below follow the v2.13.2 reference
configuration ([tagged reference configuration](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/.golangci.reference.yml),
[configuration documentation](https://golangci-lint.run/docs/configuration/file/)).

```yaml
version: "2"

linters:
  default: none
  enable:
    # Existing go vet coverage.
    - govet

    # Focused correctness and resource checks with a clean baseline.
    - bodyclose
    - durationcheck
    - nilnesserr
    - sqlclosecheck

    # Compiler-directive and suppression hygiene.
    - gocheckcompilerdirectives
    - nolintlint

  exclusions:
    generated: strict
    warn-unused: true

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  uniq-by-line: true

run:
  timeout: 5m
  relative-path-mode: gomod
  modules-download-mode: readonly
  build-tags:
    - integration
  tests: true
```

### Why this selection

This set deliberately starts with checks that add useful coverage and already
pass. `bodyclose` is relevant to the HTTP clients, `sqlclosecheck` to database
access, and `durationcheck`/`nilnesserr` to general correctness.
`gocheckcompilerdirectives` and `nolintlint` protect source-level control
comments. `govet` replaces the existing standalone command.

The `integration` build tag is specific to this repository. It loads the many
integration test files for static analysis without running them or requiring a
database. The baseline run demonstrated that this configuration can analyze
that build successfully.

Do not enable `default: all` on the first adoption. The upstream CI guidance
explicitly identifies that setting as a source of surprise build failures when
the linter set changes ([reproducibility guidance](https://golangci-lint.run/docs/welcome/install/ci/)).
After adoption, fix and enable `nilerr`, `rowserrcheck`, and `wastedassign`.
Then evaluate `staticcheck` with an explicit check policy, followed by
`errcheck`, `errorlint`, `unused`, and `ineffassign`. Evaluate `gosec`
separately because this application contains authentication, encrypted
credentials, file ingestion, and outbound HTTP. Treat style and complexity
linters as team policy decisions rather than default best practices.

### Generated code

Use `linters.exclusions.generated: strict`, not `gen/**` and
`*_templ.go` path exclusions. The v2 reference defines strict generated mode
as recognizing a line matching `^// Code generated .* DO NOT EDIT\.$` before
the first non-comment, non-blank text ([generated-file option](https://golangci-lint.run/docs/configuration/file/#linters-configuration)).
The repository's protobuf, SQLC, and templ outputs match that convention, so
the same policy covers all three generators. This is a recommendation based
on the observed headers, not an assumption that every future generator will
emit them.

Generated files are still compiled and type-checked; the setting suppresses
reported linter issues rather than making generated packages disappear from
the build. Keep generated-file verification (`make gen`, `make sqlc`, templ
generation, and `git diff --exit-code`) separate from linting because linting
does not prove that generated sources are current.

The proposed config does not enable formatters. If a formatter is adopted
later, configure its generated-file policy separately under
`formatters.exclusions.generated`; the v2 reference has separate formatter
exclusions ([formatter configuration](https://golangci-lint.run/docs/configuration/file/#formatters-configuration)).

### Runtime and issue limits

The proposed five-minute tool timeout is a project policy: it bounds a normal
local/CI lint invocation while leaving the GitHub job a larger outer budget.
The v2 configuration defines `run.timeout` as total work and allows it to be
disabled with zero ([run options](https://golangci-lint.run/docs/configuration/file/#run-configuration)).

Do not override concurrency initially. The tool chooses from the Linux
container CPU quota or logical CPU count, according to the reference
([concurrency option](https://golangci-lint.run/docs/configuration/file/#run-configuration)).

The zero issue limits prevent the default per-linter and repeated-message caps
from hiding findings. The default issue exit code is already `1`; the v2
reference documents these settings
([issues options](https://golangci-lint.run/docs/configuration/file/#issues-configuration)).

`modules-download-mode: readonly` is appropriate for CI because it makes
implicit module-file changes fail rather than silently changing dependency
metadata. The option passes `-mod=readonly` to Go package loading, as described
by the official reference ([module download mode](https://golangci-lint.run/docs/configuration/file/#run-configuration)).

## Proposed Makefile Changes

The end-state `lint` target should remain the cross-language target, but the Go
part should be delegated to the pinned executable:

```make
.PHONY: setup build test test-integration test-integration-shared lint lint-go gen templ dev clean go-tmp browser-smoke sqlc dictionary-index

GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.13.2

lint-go: go-tmp
	@$(GOLANGCI_LINT) version | grep -Fq "has version $(GOLANGCI_LINT_VERSION) " || \
		(printf '%s\n' "golangci-lint $(GOLANGCI_LINT_VERSION) is required" >&2; exit 1)
	$(GOLANGCI_LINT) run ./...

lint: lint-go
	$(VENV_BIN)/ruff check nlp/src nlp/tests
```

Remove the standalone `go vet` command rather than maintaining duplicate gates.
Keep `lint-go` whole-tree by default: the official FAQ explains that analyzing
a single file or incomplete group can produce type-check errors when dependent
files are outside the analyzed scope ([typecheck FAQ](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/welcome/faq.md#why-do-you-have-typecheck-errors)).

Do not add an automatic `curl | sh` step to `make lint`. Installation is an
environment/bootstrap concern; a missing or wrong-version executable should be
visible rather than silently downloading a tool during every lint.

## Proposed CI Changes

Add a separate Go lint job to the existing workflow. The official action
recommends a separate job because lint, build, and test can run in parallel
([official action usage](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#how-to-use)).

An end-state job, shown as a proposal, is:

```yaml
  golangci:
    name: Go lint
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: golangci-lint
        uses: golangci/golangci-lint-action@0a35821d5c230e903fcfe077583637dea1b27b47 # v9.0.0
        with:
          version: v2.13.2
```

This intentionally preserves the versions already used elsewhere in the
workflow. Pin `actions/checkout` and `actions/setup-go` by full commit SHA as a
separate workflow-hardening change if the repository adopts GitHub's immutable
action policy ([GitHub SHA guidance](https://docs.github.com/en/actions/reference/security/secure-use#using-third-party-actions)).

The existing top-level `permissions` already grants `contents: read` and
`pull-requests: read` ([`ci.yml`](../../.github/workflows/ci.yml#L11-L13)).
The proposed full-tree lint job needs only `contents: read`; the job-level
permission block intentionally narrows its token. The workflow must retain its
top-level `pull-requests: read` for the existing `dorny/paths-filter` steps.
The lint action would also need that permission in its own job if
`only-new-issues` were enabled, because that mode uses the GitHub API to obtain
pull-request or push diffs
([only-new-issues behavior](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#only-new-issues)).

### Path-filter implications

Prefer running the separate lint job on every pull request and workflow
dispatch rather than putting a workflow-level `paths` filter around it. GitHub
documents that a workflow skipped by path filtering leaves its required checks
pending, which can block merging ([workflow path-filter documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore)).
The current workflow does not have a workflow-level path filter, so adding a
job does not need to introduce this failure mode.

If the existing `dorny/paths-filter` optimization is retained for conditional
lint steps, add `.golangci.yml` to the `go` filter and keep the workflow file,
`go.mod`, `go.sum`, `Makefile`, all Go files, generated Go, protobuf, SQLC, and
migrations in the dependency trigger set. A configuration or generator change
can alter the result of a whole-tree Go lint even when no hand-written Go file
changed. This is a project-specific consequence of the current filter shown in
[`ci.yml`](../../.github/workflows/ci.yml#L28-L44).

## Local And Agent Cycle

### Normal whole-tree cycle

Agents and developers should use the repository targets, not editor-specific
or per-file invocations. A normal Go change should use:

```sh
golangci-lint version
make lint-go
go test ./...
```

The whole-tree invocation is intentional. The official FAQ says the analyzed
code should compile and warns that a partial file/package scope can lack
dependent files needed for analysis ([typecheck guidance](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/welcome/faq.md#why-do-you-have-typecheck-errors)).
Run generation before the final lint if the change affects a generator input;
the committed generated output is part of the Go tree. Run `make lint` instead
of `make lint-go` before handoff when Python was changed too. Run
`golangci-lint config verify` whenever `.golangci.yml` changes; the official
action also verifies a discovered configuration by default.

### Fast iteration and changed issues

During editing, `golangci-lint run --fast-only ./...` is a reasonable optional
feedback loop; the FAQ describes it as a development-oriented mode whose first
run may be slower while type information is cached ([fast-only FAQ](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/welcome/faq.md#why-running-with---fast-only-can-be-slow-on-the-first-run)).

Do not replace the final whole-tree run with changed `.go` files or new-issue
filtering. The recommended initial configuration has a clean baseline, so the
repository can use the simpler and stronger full-tree gate from day one.

## Staged Rollout

1. **Adopt a clean gate:** add the proposed config, `lint-go` target, README and
   agent instructions, and separate CI job. Remove standalone `go vet` from the
   Makefile and CI, and update the definition of done in
   [`doc/design/roadmap.md`](../design/roadmap.md#definition-of-done-for-each-issue)
   so it does not retain a duplicate requirement. Validate both normal and
   integration-tagged source loading.
2. **Clear the five smallest correctness findings:** review the three `nilerr`,
   one `rowserrcheck`, and one `wastedassign` findings, then enable those
   linters in the same cleanup changes.
3. **Tune staticcheck:** select the desired check classes, fix the accepted
   findings, and only then enable it. Do not blanket-disable all findings in a
   directory.
4. **Audit error/resource handling:** handle `errcheck` and `errorlint` in
   dedicated changes. Add narrowly scoped exclusions only where ignored
   cleanup errors or non-wrapping are intentional. Require explanations for
   source suppressions through `nolintlint`.
5. **Review dead code:** fix and enable `unused` and `ineffassign`; account for
   the additional integration-tag findings.
6. **Decide optional policy:** independently evaluate `gosec`, test-style,
   modernization, complexity, and naming linters. Their availability is not a
   reason to enforce them.
7. **Maintenance:** update the exact linter patch and action SHA through a
   reviewed dependency change. Re-run the full verification commands and
   inspect the diff before accepting new diagnostics.

## Tradeoffs

| Choice | Benefit | Cost or risk |
| --- | --- | --- |
| Exact `v2.13.2` binary | Reproducible local and CI diagnostics | Requires deliberate upgrades and binary installation outside `go.mod` |
| v2 schema with `default: none` | Stable, reviewable policy; no surprise linter additions | Maintainers must decide when to add a linter |
| Strict generated-file mode | Covers protobuf, SQLC, and templ without brittle directory lists | A generator that omits the standard header will not be recognized automatically |
| Separate action job | Parallel feedback and native annotations | Adds a CI job and a second tool cache |
| Clean, focused initial set | Full-tree enforcement works immediately | Broader existing defects are not gated until their linter is enabled |
| `integration` build tag | Static analysis covers integration-only source without services | More packages and test code increase lint time |
| Automatic concurrency | Adapts to developer and CI CPU limits | Runtime is less identical across machines |

The action's annotations are tied to its default text output and require
`contents: read`; pull-request read access is additionally needed for
`only-new-issues` ([action annotations and permissions](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md#annotations)).

## Verification Commands

After a future implementation, verify the integration in this order:

```sh
golangci-lint version
golangci-lint config verify
golangci-lint linters
GOTOOLCHAIN=go1.24.0 golangci-lint run ./...
make build
make test
make lint
```

If generator inputs changed, regenerate the relevant artifacts before linting
and inspect their diff. For one-time adoption comparison:

```sh
go vet ./...
go vet -tags=integration ./...
golangci-lint run --enable-only=govet ./...
```

The CLI provides `config verify`, `linters`, `version`, `run`, and the
`--enable-only` option ([CLI documentation](https://golangci-lint.run/docs/configuration/cli/)).

## Sources

All external claims in this report use primary sources from golangci-lint,
its official GitHub Action, GitHub Actions, or the official setup-go action:

- [golangci-lint v2.13.2 release API](https://api.github.com/repos/golangci/golangci-lint/releases/tags/v2.13.2)
- [golangci-lint v2.13.2 release page](https://github.com/golangci/golangci-lint/releases/tag/v2.13.2)
- [golangci-lint v2.13.2 `go.mod`](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/go.mod)
- [golangci-lint v2.13.2 FAQ](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/welcome/faq.md)
- [golangci-lint v2.13.2 migration guide](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/docs/content/docs/product/migration-guide.md)
- [golangci-lint v2.13.2 reference configuration](https://raw.githubusercontent.com/golangci/golangci-lint/v2.13.2/.golangci.reference.yml)
- [Current official configuration documentation](https://golangci-lint.run/docs/configuration/file/)
- [Current official CLI documentation](https://golangci-lint.run/docs/configuration/cli/)
- [Current official local installation documentation](https://golangci-lint.run/docs/welcome/install/local/)
- [Current official CI installation documentation](https://golangci-lint.run/docs/welcome/install/ci/)
- [Current official linter list](https://golangci-lint.run/docs/linters/)
- [golangci-lint-action v9.0.0 release](https://github.com/golangci/golangci-lint-action/releases/tag/v9.0.0)
- [golangci-lint-action v9.0.0 README at its release commit](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/README.md)
- [golangci-lint-action v9.0.0 metadata at its release commit](https://raw.githubusercontent.com/golangci/golangci-lint-action/0a35821d5c230e903fcfe077583637dea1b27b47/action.yml)
- [actions/setup-go v6 README](https://raw.githubusercontent.com/actions/setup-go/v6.0.0/README.md)
- [actions/setup-go v6 release](https://github.com/actions/setup-go/releases/tag/v6.0.0)
- [GitHub Actions workflow path filters](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore)
- [GitHub Actions secure use and SHA pinning](https://docs.github.com/en/actions/reference/security/secure-use#using-third-party-actions)
