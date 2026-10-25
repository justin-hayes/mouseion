#!/usr/bin/env bash
# Tests tools/check-dockerfile-go.sh against fixture go.mod/Dockerfile pairs.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/check-dockerfile-go.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
failures=0

# expect <pass|fail> <description> <go directive line> <FROM line>
expect() {
  local want=$1 name=$2
  printf 'module example\n\n%s\n' "$3" > "$work/go.mod"
  printf '# syntax\n%s\nWORKDIR /src\n' "$4" > "$work/Dockerfile"
  local got=pass
  "$script" "$work/go.mod" "$work/Dockerfile" > /dev/null 2>&1 || got=fail
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: $name: got $got, want $want" >&2
    failures=$((failures + 1))
  fi
}

expect pass "equal versions" "go 1.27.3" "FROM golang:1.27.3 AS build"
expect pass "newer image" "go 1.27.3" "FROM golang:1.27.4 AS build"
expect pass "newer minor image" "go 1.27.3" "FROM golang:1.28.0"
expect pass "image variant suffix" "go 1.27.3" "FROM golang:1.27.3-bookworm AS build"
expect pass "patch 10 sorts after patch 9" "go 1.27.9" "FROM golang:1.27.10"
expect fail "older image" "go 1.27.3" "FROM golang:1.27.2 AS build"
expect fail "older minor image" "go 1.28.0" "FROM golang:1.27.9"
expect fail "unversioned image" "go 1.27.3" "FROM golang:latest AS build"
expect fail "no golang image" "go 1.27.3" "FROM debian:trixie"
expect fail "no go directive" "toolchain go1.27.3" "FROM golang:1.27.3"

if ((failures > 0)); then
  exit 1
fi
echo "check-dockerfile-go: all cases pass"
