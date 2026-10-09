#!/usr/bin/env bash
# Behavior tests for the CI change classifier and the Browser smoke gate.
#
# Seams: tools/ci-changes.sh (changed paths on stdin -> GitHub outputs) and
# tools/ci-gate.sh (job results -> exit status). The expected job selection for
# each representative change class is documented in doc/development.md.
set -euo pipefail

cd "$(dirname "$0")/.."

CLASSIFIER=tools/ci-changes.sh
GATE=tools/ci-gate.sh
ALL="browser build frontend generated go python"
failures=0

# normalize prints the words of its argument sorted and space-separated, so
# expectations can be written in any order.
normalize() {
  printf '%s\n' $1 | sed '/^$/d' | sort | tr '\n' ' ' | sed 's/ $//'
}

# expect <name> <expected true outputs> <changed paths...>
expect() {
  local name=$1 want got
  want=$(normalize "$2")
  shift 2
  got=$(printf '%s\n' "$@" | "$CLASSIFIER" 2>/dev/null | grep '=true$' | sed 's/=true$//' || true)
  got=$(normalize "$got")
  if [ "$got" = "$want" ]; then
    echo "ok   $name"
  else
    echo "FAIL $name: want [$want] got [$got]"
    failures=$((failures + 1))
  fi
}

# gate <name> <pass|fail> <changes result> <browser required> <shards result>
gate() {
  local name=$1 want=$2 got
  shift 2
  if "$GATE" "$@" >/dev/null 2>&1; then got=pass; else got=fail; fi
  if [ "$got" = "$want" ]; then
    echo "ok   $name"
  else
    echo "FAIL $name: want $want got $got"
    failures=$((failures + 1))
  fi
}

echo "== classification"
expect "docs-only change runs no check" "" \
  doc/features/reading-workflow.md README.md
expect "nlp-only change runs python only" "build python" \
  nlp/src/mouseion_nlp/server.py nlp/tests/test_server.py
expect "screenshot-only change runs no check" "" \
  e2e/screenshot/run.mjs e2e/screenshot/manifest.json e2e/screenshot/known-vocabulary.ts
expect "shared npm dependency runs browser" "browser" \
  e2e/package-lock.json
expect "shared Playwright config runs browser" "browser" \
  e2e/playwright.config.ts
expect "browser test runs browser" "browser" \
  e2e/tests/smoke.spec.ts
expect "browser support code runs browser" "browser" \
  e2e/support/test.ts
expect "frontend template runs browser, Go, and frontend checks" "browser build frontend go" \
  internal/webapp/layout.templ
expect "frontend stylesheet source runs frontend checks only" "build frontend" \
  internal/webapp/styles/app.css
expect "embedded static asset runs browser, Go, and frontend checks" "browser build frontend go" \
  internal/webapp/static/my-books.js
expect "fixture server source runs browser and Go" "browser build go" \
  cmd/fixtureserver/main.go
expect "shared Go package used by fixture server runs browser and Go" "browser build go" \
  internal/domain/books.go
expect "embedded Go data runs browser and Go" "browser build go" \
  internal/canonicalization/german_post1996.json
expect "Go-only test runs Go only" "build go" \
  internal/domain/books_test.go
expect "Go-only testdata runs Go only" "build go" \
  internal/epub/testdata/OEBPS/content.opf
expect "test helper runs Go only" "build go" \
  internal/testutil/postgres.go
expect "server command runs Go only" "build go" \
  cmd/server/main.go
expect "Go lint configuration runs Go only" "build go" \
  .golangci.yml
expect "NLP expectation read by Go tests runs Go and python" "build go python" \
  nlp/tests/testdata/italian_stanza_expected.json
expect "NLP data read by Go canonicalization tests runs Go and python" "build go python" \
  nlp/testdata/german_normalization_parity.jsonl
expect "migration runs generated, Go, and browser checks" "browser build generated go" \
  migrations/000002_example.up.sql
expect "stylesheet class source runs frontend, Go, and browser checks" "browser build frontend go" \
  internal/webapp/components.go
expect "protobuf source runs generated, Go, and python checks" "build generated go python" \
  proto/mouseion/v1/normalized_corpus.proto
expect "sqlc query runs generated and Go checks" "build generated go" \
  sqlc/queries/books.sql
expect "generated Go output runs generated, Go, and browser checks" "browser build generated go" \
  gen/go/mouseion/v1/normalized_corpus.pb.go
expect "generated Python output runs generated and python checks" "build generated python" \
  gen/python/mouseion/v1/normalized_corpus_pb2.py
expect "Python dependency manifest runs generated and python checks" "build generated python" \
  nlp/requirements-dev.txt
expect "build tooling runs every check" "$ALL" \
  Makefile
expect "tools change runs every check" "$ALL" \
  tools/ci-changes.sh
expect "workflow change runs every check" "$ALL" \
  .github/workflows/ci.yml
expect "unclassified path runs every check" "$ALL" \
  Dockerfile
expect "docs mixed with fixture source still runs browser and Go" "browser build go" \
  doc/features/reading-workflow.md cmd/fixtureserver/main.go
expect "empty change set runs no check" "" \
  ""

echo "== manual dispatch"
dispatch=$("$CLASSIFIER" --all 2>/dev/null | grep '=true$' | sed 's/=true$//' || true)
if [ "$(normalize "$dispatch")" = "$(normalize "$ALL")" ]; then
  echo "ok   manual dispatch runs every check"
else
  echo "FAIL manual dispatch runs every check: got [$dispatch]"
  failures=$((failures + 1))
fi

echo "== workflow wiring"
workflow=.github/workflows/ci.yml
classes="build go python generated frontend browser"

for class in $classes; do
  if grep -q "^      $class: \${{ steps.classify.outputs.$class }}\$" "$workflow"; then
    echo "ok   changes job exposes $class"
  else
    echo "FAIL changes job does not expose $class"
    failures=$((failures + 1))
  fi
done

for referenced in $(grep -o 'needs\.changes\.outputs\.[a-z]*' "$workflow" | sed 's/.*\.//' | sort -u); do
  case " $classes " in
    *" $referenced "*) ;;
    *)
      echo "FAIL workflow reads change output '$referenced', which the changes job does not expose"
      failures=$((failures + 1))
      ;;
  esac
done

# job_if prints the job-level if: line of one job.
job_if() {
  awk -v job="$1" '
    $0 == "  " job ":" { inside = 1; next }
    inside && /^  [a-z-]+:/ { exit }
    inside && /^    if:/ { print; exit }
  ' "$workflow"
}

# gated <job> <class> <guarded: yes|no>: the job runs for its class; a guarded
# job also runs, and fails at its guard step, when change detection failed.
gated() {
  local job=$1 class=$2 line
  line=$(job_if "$job")
  if [[ $line != *"needs.changes.outputs.$class == 'true'"* ]]; then
    echo "FAIL $job is not gated on $class"
    failures=$((failures + 1))
  elif [ "$3" = yes ] && [[ $line != *"!cancelled()"* || $line != *"needs.changes.result != 'success'"* ]]; then
    echo "FAIL $job does not run and fail when change detection failed"
    failures=$((failures + 1))
  else
    echo "ok   $job is gated on $class"
  fi
}

gated ci build yes
gated golangci go yes
gated integration go yes
gated browser-fixture browser yes
gated browser-smoke-shard browser no

guards=$(grep -c 'Fail when change detection failed' "$workflow" || true)
if [ "$guards" -eq 4 ]; then
  echo "ok   four guarded jobs fail when change detection failed"
else
  echo "FAIL expected 4 change-detection guard steps, found $guards"
  failures=$((failures + 1))
fi

if command -v actionlint >/dev/null 2>&1; then
  if actionlint "$workflow"; then
    echo "ok   actionlint accepts $workflow"
  else
    echo "FAIL actionlint rejects $workflow"
    failures=$((failures + 1))
  fi
else
  echo "skip actionlint not on PATH"
fi

echo "== browser smoke gate"
gate "shards pass when browser changed" pass success true success
gate "shard failure fails the gate" fail success true failure
gate "shard cancellation fails the gate" fail success true cancelled
gate "skipped shards fail when browser changed" fail success true skipped
gate "skipped shards pass when browser unchanged" pass success false skipped
gate "shards that ran fail when browser unchanged" fail success false success
gate "failed change detection fails the gate" fail failure true failure
gate "failed change detection fails even when shards skipped" fail failure false skipped
gate "cancelled change detection fails the gate" fail cancelled false skipped

if [ "$failures" -ne 0 ]; then
  echo "$failures check(s) failed"
  exit 1
fi
echo "all CI change checks passed"
