#!/usr/bin/env bash
# Maps changed paths to the CI check classes that must run.
#
# Reads one changed path per line on stdin, or runs with --all for manual
# dispatch. Prints GitHub Actions outputs (key=true|false) on stdout and a
# per-path reason on stderr. Any path that matches no rule runs every class,
# so a new input fails safe. The rules and the expected job selection for each
# change class are documented in doc/development.md ("CI change classes");
# update both together and extend tools/ci-changes_test.sh.
#
# Classes:
#   go         Go build, Go test, Go lint, and integration tests
#   python     NLP build, test, and lint
#   generated  regeneration of protobuf, sqlc, and the committed gen/ outputs
#   frontend   stylesheet generation and template/static asset checks
#   browser    the Playwright browser smoke suite and the fixture server it runs
#   build      derived: any of go, python, generated, or frontend
set -euo pipefail

CLASSES=(go python generated frontend browser)
declare -A on=()

mark() {
  local class
  for class in "$@"; do on[$class]=1; done
}

mark_all() {
  mark "${CLASSES[@]}"
}

# classify records which classes one changed path affects.
classify() {
  local path=$1
  case "$path" in
    # Documentation and the screenshot-only project are not inputs to any check.
    doc/* | *.md | CITATION.cff | LICENSE | SECURITY.md | e2e/screenshot/*)
      echo "docs/screenshot: $path" >&2 ;;

    # Build tooling and the workflow can change how every check behaves.
    Makefile | tools/* | .github/workflows/*)
      mark_all
      echo "build tooling: $path" >&2 ;;

    # Go tests read these NLP expectations at runtime.
    nlp/tests/testdata/* | nlp/testdata/*)
      mark go python
      echo "nlp data read by Go tests: $path" >&2 ;;
    nlp/requirements*.txt | nlp/pyproject.toml)
      mark python generated
      echo "python dependencies: $path" >&2 ;;
    nlp/*)
      mark python
      echo "nlp: $path" >&2 ;;

    # Browser suite inputs. Shared npm configuration is browser-only; the
    # screenshot project's dependency on it is covered by the rule above.
    e2e/tests/* | e2e/support/* | e2e/playwright.config.ts | e2e/package.json | e2e/package-lock.json)
      mark browser
      echo "browser suite: $path" >&2 ;;

    # Go test inputs never reach a binary, so they only run Go checks.
    *_test.go | internal/analyzer/analyzertest/* | internal/testutil/* | internal/testwrite/* | */testdata/* | */testfixtures/*)
      mark go
      echo "go test input: $path" >&2 ;;

    # The fixture server is the browser suite's binary.
    cmd/fixtureserver/*)
      mark go browser
      echo "fixture server: $path" >&2 ;;
    cmd/*)
      mark go
      echo "go command: $path" >&2 ;;

    # Embedded frontend assets and templates are compiled into the server.
    internal/webapp/styles/*)
      mark frontend
      echo "stylesheet source: $path" >&2 ;;
    internal/webapp/static/*)
      mark go frontend browser
      echo "embedded static asset: $path" >&2 ;;
    *.templ)
      mark go frontend browser
      echo "template: $path" >&2 ;;
    internal/webapp/components.go)
      mark go frontend browser
      echo "stylesheet class source: $path" >&2 ;;
    internal/canonicalization/german_post1996.json | internal/cardexport/templates/*)
      mark go browser
      echo "embedded data: $path" >&2 ;;

    # Generated code is committed, so each output is checked against its generator.
    gen/go/* | gen/sqlc/*)
      mark go browser generated
      echo "generated Go: $path" >&2 ;;
    gen/python/*)
      mark python generated
      echo "generated Python: $path" >&2 ;;
    proto/*)
      mark generated go python
      echo "generator input (gen/go and gen/python are its outputs): $path" >&2 ;;
    sqlc/* | sqlc.yaml)
      mark generated go
      echo "generator input (gen/sqlc is its output): $path" >&2 ;;
    migrations/*)
      mark go browser generated
      echo "migration (embedded and sqlc input): $path" >&2 ;;
    go.mod | go.sum)
      mark go browser generated
      echo "Go dependencies: $path" >&2 ;;
    .golangci.yml)
      mark go
      echo "Go lint configuration: $path" >&2 ;;

    # The fixture server's transitive internal packages, which the binary compiles.
    internal/*)
      mark go browser
      echo "internal package: $path" >&2 ;;

    *)
      mark_all
      echo "unclassified, running every check: $path" >&2 ;;
  esac
}

if [ "${1:-}" = "--all" ]; then
  mark_all
  echo "manual dispatch: running every check" >&2
else
  while IFS= read -r path; do
    [ -n "$path" ] || continue
    classify "$path"
  done
fi

for class in "${CLASSES[@]}"; do
  if [ -n "${on[$class]:-}" ]; then echo "$class=true"; else echo "$class=false"; fi
done
if [ -n "${on[go]:-}${on[python]:-}${on[generated]:-}${on[frontend]:-}" ]; then
  echo "build=true"
else
  echo "build=false"
fi
