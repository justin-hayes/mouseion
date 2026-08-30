# Integration CI job (change-detection gated)

Add the following job under `jobs:` in `.github/workflows/ci.yml`, alongside
the existing `ci` job. It mirrors the existing change-detection pattern: the
job only runs when a PR touches files that could affect the Go/integration
surface, so it does **not** fire on every PR (doc-only, NLP-only, infra-only
changes skip it).

Two things to copy from the existing `ci` job:

1. The `Detect changed files` step uses `dorny/paths-filter` with the same
   `go` filter (already defined in the workflow's `ci` job — reuse that block
   as written; do not duplicate differently). The gate is
   `steps.changes.outputs.go == 'true'`.
2. The `Set up Go` / `Go integration test` steps run with the
   `github.event_name == 'workflow_dispatch'` OR `steps.changes.outputs.go == 'true'`
   guard, matching how the existing `Go test` step is gated.

The job uses the Docker service already available on an `ubuntu-latest` runner
and runs the PostgreSQL-backed integration packages that are otherwise excluded
by the default, untagged test command.

```yaml
  integration:
    name: Integration tests
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Detect changed files
        id: changes
        if: github.event_name == 'pull_request'
        uses: dorny/paths-filter@v3
        with:
          filters: |
            go:
              - '**/*.go'
              - 'go.mod'
              - 'go.sum'
              - 'proto/**'
              - 'gen/go/**'
              - 'Makefile'
              - '.github/workflows/ci.yml'

      - name: Set up Go
        if: |
          github.event_name == 'workflow_dispatch' ||
          steps.changes.outputs.go == 'true'
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Go integration test
        if: |
          github.event_name == 'workflow_dispatch' ||
          steps.changes.outputs.go == 'true'
        run: go test -tags=integration ./internal/persistence ./internal/prepareddeck ./internal/webapp
```

## Notes

- No `MOUSEION_TEST_DATABASE_URL` should be configured for this job: the tests
  use Testcontainers and the Docker socket to provision PostgreSQL.
- This is **PR-gated** so defective integration code is caught *before* it
  reaches `main`, while non-Go PRs skip the (slow) integration run entirely.
- This job is intentionally documented here — not applied to
  `.github/workflows/ci.yml` — because applying it requires a workflow-scoped
  push permission that the automation token does not have.