#!/usr/bin/env bash
# Behavior tests for the Go coverage profile filter.
#
# Seam: tools/coverage-filter.sh (Go cover profile on stdin -> filtered profile
# on stdout). Generated code is removed; hand-written lines pass through
# unchanged, and the mode header is always kept.
set -euo pipefail

cd "$(dirname "$0")/.."

FILTER=tools/coverage-filter.sh
MODULE=github.com/justin-hayes/mouseion
failures=0

# expect <name> <expected output> <profile...>
expect() {
  local name=$1 want=$2 got
  shift 2
  if got=$("$FILTER" <<<"$(printf '%s\n' "$@")"); then
    if [[ "$got" == "$want" ]]; then
      echo "ok   $name"
      return
    fi
    echo "FAIL $name:"
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") | sed 's/^/     /' || true
  else
    echo "FAIL $name: filter exited non-zero"
  fi
  failures=$((failures + 1))
}

# A representative profile: generated protobuf, sqlc, and templ code mixed
# with hand-written packages, including names that only look generated.
profile=(
  "mode: set"
  "$MODULE/gen/go/mouseion/v1/normalized_corpus.pb.go:20.2,25.3 3 1"
  "$MODULE/gen/sqlc/books.sql.go:14.2,18.3 2 0"
  "$MODULE/internal/webapp/layout_templ.go:40.2,44.3 2 0"
  "$MODULE/internal/domain/books.go:10.2,12.3 1 1"
  "$MODULE/internal/domain/books.go:14.2,16.3 1 0"
  "$MODULE/internal/generation/plan.go:8.2,9.3 1 1"
  "$MODULE/internal/webapp/general_templ_helpers.go:3.2,4.3 1 1"
)

echo "== coverage profile filtering"
expect "generated lines are removed; hand-written lines are kept unchanged" \
  "mode: set
$MODULE/internal/domain/books.go:10.2,12.3 1 1
$MODULE/internal/domain/books.go:14.2,16.3 1 0
$MODULE/internal/generation/plan.go:8.2,9.3 1 1
$MODULE/internal/webapp/general_templ_helpers.go:3.2,4.3 1 1" \
  "${profile[@]}"

expect "mode header is kept" \
  "mode: atomic" \
  "mode: atomic"

expect "only generated lines yield a header-only profile" \
  "mode: set" \
  "mode: set" \
  "$MODULE/gen/go/mouseion/v1/normalized_corpus.pb.go:20.2,25.3 3 1" \
  "$MODULE/internal/webapp/layout_templ.go:40.2,44.3 2 0"

if [[ "$failures" -ne 0 ]]; then
  echo "$failures check(s) failed"
  exit 1
fi
echo "all coverage filter checks passed"
