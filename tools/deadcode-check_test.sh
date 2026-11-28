#!/usr/bin/env bash
# Tests tools/deadcode-check.sh against fixture reports and allowlists.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/deadcode-check.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0

# expect <pass|fail> <description> <allowlist> <report>
expect() {
  local want=$1 name=$2
  printf '%b' "$3" > "$work/allowlist"
  local got=pass
  printf '%b' "$4" | "$script" "$work/allowlist" > /dev/null 2>&1 || got=fail
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: $name: got $got, want $want" >&2
    failures=$((failures + 1))
  fi
}

expect pass "empty report, empty allowlist" "" ""
expect pass "allowlisted report" "# c\n\na.F kept for tests\n" "a.F a/f.go\n"
expect pass "gen/ reports ignored" "" "a.Register gen/go/x.pb.go\n"
expect pass "method entry" "a.T.M reason\n" "a.T.M a/t.go\n"
expect fail "unlisted report" "" "a.F a/f.go\n"
expect fail "unlisted templ component" "" "a.Page internal/webapp/x_templ.go\n"
expect fail "stale entry" "a.F kept for tests\n" ""
expect fail "entry without reason" "a.F\n" "a.F a/f.go\n"
expect fail "one of two unlisted" "a.F r\n" "a.F a/f.go\na.G a/g.go\n"

if ((failures > 0)); then
  exit 1
fi
echo "deadcode-check: ok"
