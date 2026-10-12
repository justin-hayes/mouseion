#!/usr/bin/env bash
# Counts the new lines of code a change adds, the way SonarQube Cloud counts
# them for its "fewer than 20 new lines" rule (tools/coverage-new.sh).
#
# Usage: tools/new-code-lines.sh <base commit>
#
# Run from the root of the repository to count. Compares the working tree,
# staged and unstaged, with <base commit>, as diff-cover does. A line counts
# when it is added, not blank, and not a comment-only line, in a file
# SonarQube Cloud analyses as source: a file type it reports lines of code for,
# minus sonar.exclusions and sonar.test.inclusions in sonar-project.properties.
# The behavior is tested by tools/coverage-new_test.sh.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <base commit>" >&2
  exit 2
fi
base=$1

# The file types SonarQube Cloud reports lines of code for in this project.
pathspecs=(
  ':(glob)**/*.go' ':(glob)**/*.py' ':(glob)**/*.js' ':(glob)**/*.css'
  ':(glob)**/*.html' ':(glob)**/*.sql' ':(glob)**/*.sh' ':(glob)**/*.yml'
  ':(glob)**/*.yaml' ':(glob)**/Dockerfile'
)

# Sonar's ** wildcards mean the same in a Git glob pathspec.
property() {
  sed -n "s/^$1=//p" sonar-project.properties
}
for key in 'sonar\.exclusions' 'sonar\.test\.inclusions'; do
  IFS=, read -r -a patterns <<<"$(property "$key")"
  for pattern in "${patterns[@]}"; do
    [[ -n "$pattern" ]] && pathspecs+=(":(glob,exclude)$pattern")
  done
done

git diff --no-color --no-ext-diff --find-renames -U0 "$base" -- "${pathspecs[@]}" | awk '
/^\+\+\+ / { next }
/^\+/ {
  line = substr($0, 2)
  sub(/^[ \t]+/, "", line)
  if (line == "" || line ~ /^(\/\/|\/\*|#|--)/) next
  count++
}
END { print count + 0 }
'
