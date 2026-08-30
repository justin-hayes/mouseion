# Integration CI job

Add the following job under `jobs:` in `.github/workflows/ci.yml`, alongside
the existing `ci` job. It uses the Docker service already available on an
`ubuntu-latest` runner and runs the PostgreSQL-backed integration packages
that are otherwise excluded by the default, untagged test command.

```yaml
  integration:
    name: Integration tests
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Go integration test
        run: go test -tags=integration ./internal/persistence ./internal/prepareddeck ./internal/webapp
```

No `MOUSEION_TEST_DATABASE_URL` should be configured for this job: the tests
use Testcontainers and the Docker socket to provision PostgreSQL. This job is
intentionally documented here because applying it to the workflow requires a
workflow-scoped push permission.
