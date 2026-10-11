#!/usr/bin/env bash
# Runs the Go unit and integration tests with coverage profiles and writes the
# filtered profiles to the fixed report directory .tmp/coverage (gitignored).
# SonarQube Cloud reads both (sonar-project.properties) and merges them.
#
# The integration run needs the same PostgreSQL prerequisites as
# `make test-integration`: Docker for Testcontainers, or MOUSEION_TEST_DATABASE_URL
# pointing at a role with CREATEDB. Callers must have run make templ first;
# make coverage does.
#
# -coverpkg=./... credits integration coverage to every module package the
# tests execute (for example the PostgreSQL store), not only to the package
# under test.
set -euo pipefail

cd "$(dirname "$0")/.."

report_dir=.tmp/coverage
unit_raw="$report_dir/go-unit.raw"
unit_report="$report_dir/go-unit.out"
integration_raw="$report_dir/go-integration.raw"
integration_report="$report_dir/go-integration.out"

mkdir -p "$report_dir"

go test -count=1 -coverprofile="$unit_raw" ./...
tools/coverage-filter.sh <"$unit_raw" >"$unit_report"
rm -f "$unit_raw"
printf 'Go unit-test coverage profile: %s\n' "$unit_report"

go test -count=1 -tags=integration -coverpkg=./... -coverprofile="$integration_raw" ./internal/...
tools/coverage-filter.sh <"$integration_raw" >"$integration_report"
rm -f "$integration_raw"
printf 'Go integration-test coverage profile: %s\n' "$integration_report"
