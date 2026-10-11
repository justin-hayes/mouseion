#!/usr/bin/env bash
# Runs the Go unit tests with a coverage profile and writes the filtered
# profile to the fixed report directory .tmp/coverage (gitignored). SonarQube
# Cloud reads go-unit.out (sonar-project.properties). Callers must have run
# make templ first; make coverage does.
set -euo pipefail

cd "$(dirname "$0")/.."

report_dir=.tmp/coverage
raw="$report_dir/go-unit.raw"
report="$report_dir/go-unit.out"

mkdir -p "$report_dir"
go test -count=1 -coverprofile="$raw" ./...
tools/coverage-filter.sh <"$raw" >"$report"
rm -f "$raw"
printf 'Go unit-test coverage profile: %s\n' "$report"
