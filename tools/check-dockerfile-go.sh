#!/usr/bin/env bash
# Fails when the Dockerfile's Go build image is older than go.mod's go
# directive. The official golang images set GOTOOLCHAIN=local, so an older
# image cannot build the module and the container build fails at
# `go mod download`. CI's Go jobs read their version from go.mod and never see
# the mismatch. A newer image is fine; Dependabot bumps it on its own.
#
# Usage: tools/check-dockerfile-go.sh [go.mod] [Dockerfile]
set -euo pipefail

gomod=${1:-go.mod}
dockerfile=${2:-Dockerfile}

required=$(sed -nE 's/^go ([0-9][0-9.]*)[[:space:]]*$/\1/p' "$gomod")
image=$(sed -nE 's/^FROM golang:([0-9][0-9.]*)([-@[:space:]].*)?$/\1/p' "$dockerfile")

if [[ -z "$required" ]]; then
  echo "no go directive found in $gomod" >&2
  exit 1
fi
if [[ -z "$image" ]]; then
  echo "no versioned FROM golang:<version> line found in $dockerfile" >&2
  exit 1
fi

if [[ "$(printf '%s\n%s\n' "$required" "$image" | sort -V | head -n1)" != "$required" ]]; then
  echo "$dockerfile builds with golang:$image, but $gomod requires go >= $required." >&2
  echo "Bump the FROM golang image in $dockerfile to at least $required." >&2
  exit 1
fi
echo "$dockerfile golang:$image satisfies $gomod go $required"
