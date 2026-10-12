#!/usr/bin/env bash
# Behavior tests for the new-code line count behind tools/coverage-new.sh.
#
# Seam: tools/new-code-lines.sh (a Git repository and a base commit -> the
# number of new lines of code). Each case builds a scratch repository under
# .tmp, commits a base, changes the working tree, and checks the count.
set -euo pipefail

cd "$(dirname "$0")/.."

COUNTER=$PWD/tools/new-code-lines.sh
mkdir -p .tmp
scratch=$(mktemp -d "$PWD/.tmp/coverage-new-test.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
failures=0

# repo <name> creates a repository with Sonar settings and one base commit, and
# prints its path.
repo() {
  local dir=$scratch/$1
  mkdir -p "$dir"
  git -C "$dir" init -q
  cat >"$dir/sonar-project.properties" <<'EOF'
sonar.exclusions=gen/**,**/*_templ.go
sonar.test.inclusions=**/*_test.go,nlp/tests/**
EOF
  printf 'package a\n' >"$dir/a.go"
  git -C "$dir" add -A
  git -C "$dir" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false commit -q -m base
  printf '%s\n' "$dir"
}

# expect <name> <want> <dir>
expect() {
  local name=$1 want=$2 got
  if got=$(cd "$3" && "$COUNTER" HEAD); then
    if [[ "$got" == "$want" ]]; then
      echo "ok   $name"
      return
    fi
    echo "FAIL $name: want $want got $got"
  else
    echo "FAIL $name: counter exited non-zero"
  fi
  failures=$((failures + 1))
}

echo "== new-code line count"

dir=$(repo code)
printf 'package a\n\nfunc f() int {\n\t// why\n\n\treturn 1\n}\n' >"$dir/a.go"
expect "added code lines count; blank and comment lines do not" 3 "$dir"

dir=$(repo staged)
printf 'select 1;\n-- note\n' >"$dir/q.sql"
printf '# heading\n' >"$dir/run.sh"
git -C "$dir" add -A
expect "staged files count, with their comment syntax" 1 "$dir"

dir=$(repo excluded)
mkdir -p "$dir/gen" "$dir/nlp/tests"
printf 'package gen\nvar x = 1\n' >"$dir/gen/x.go"
printf 'package a\nvar y = 1\n' >"$dir/a_templ.go"
printf 'package a\nfunc TestA() {}\n' >"$dir/a_test.go"
printf 'def test_a():\n    pass\n' >"$dir/nlp/tests/test_a.py"
git -C "$dir" add -A
expect "excluded and test files do not count" 0 "$dir"

dir=$(repo unanalysed)
printf '# Title\ntext\n' >"$dir/README.md"
printf '{"a": 1}\n' >"$dir/data.json"
git -C "$dir" add -A
expect "file types Sonar reports no lines of code for do not count" 0 "$dir"

dir=$(repo untracked)
printf 'x = 1\n' >"$dir/b.py"
expect "untracked files do not count" 0 "$dir"

if ((failures > 0)); then
  echo "$failures failure(s)"
  exit 1
fi
