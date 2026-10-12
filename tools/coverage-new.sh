#!/usr/bin/env bash
# Previews the SonarQube Cloud quality-gate condition on coverage (80% on new
# code) for the lines a change adds, so an author sees uncovered lines before
# opening a pull request. SonarQube Cloud's gate stays the authority; this is a
# local preview and does not run in CI.
#
# Reads the reports `make coverage` writes to .tmp/coverage. The Go profiles are
# converted to Cobertura XML with gocover-cobertura (a go.mod tool) and checked
# together with the Python report by diff-cover (the dev group in
# nlp/pyproject.toml). A line counts as covered when any report covers it, as in
# SonarQube Cloud.
#
# The change is the working tree, staged and unstaged, compared with its merge
# base on COVERAGE_BASE (default origin/main). Untracked files are not part of
# it; stage new files first. Like SonarQube Cloud, the check passes whenever the
# change adds fewer than 20 new lines of code (tools/new-code-lines.sh).
#
# Known differences from SonarQube Cloud: Sonar decides which lines are new
# with its own diff, so the line counts can differ by a few lines; and files
# outside the coverage reports (tools, and other Python and JavaScript) count
# as uncovered in Sonar but are not checked here.
set -euo pipefail

cd "$(dirname "$0")/.."

base=${COVERAGE_BASE:-origin/main}
report_dir=.tmp/coverage
min_coverage=80
min_new_lines=20

for report in go-unit.out go-integration.out python.xml; do
  if [[ ! -f "$report_dir/$report" ]]; then
    echo "coverage-new: $report_dir/$report is missing; run make coverage first." >&2
    exit 2
  fi
done

merge_base=$(git merge-base "$base" HEAD)

# gocover-cobertura loads the profile's packages, so the integration profile
# needs the integration build tag to resolve the files behind it.
go tool gocover-cobertura <"$report_dir/go-unit.out" >"$report_dir/go-unit.xml"
GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=integration" \
  go tool gocover-cobertura <"$report_dir/go-integration.out" >"$report_dir/go-integration.xml"

new_lines=$(tools/new-code-lines.sh "$merge_base")

args=(
  "$report_dir/go-unit.xml" "$report_dir/go-integration.xml" "$report_dir/python.xml"
  --compare-branch="$merge_base"
)

printf 'New code since %s: %d lines\n' "$(git rev-parse --short "$merge_base")" "$new_lines"
if ((new_lines < min_new_lines)); then
  .venv/bin/diff-cover "${args[@]}"
  printf 'Fewer than %d new lines: SonarQube Cloud does not apply the coverage condition.\n' "$min_new_lines"
else
  .venv/bin/diff-cover "${args[@]}" --fail-under="$min_coverage"
fi
