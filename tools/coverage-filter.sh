#!/usr/bin/env bash
# Removes generated Go code from a coverage profile.
#
# Reads a Go cover profile on stdin and writes the filtered profile on stdout.
# Lines for files under a gen/ directory (protobuf, sqlc) and for *_templ.go
# files are dropped; every other line, and the mode header, is kept unchanged.
# The behavior is tested by tools/coverage-filter_test.sh.
set -euo pipefail

awk '
NR == 1 {
  if ($0 !~ /^mode: /) {
    print "coverage-filter: first line is not a mode header: " $0 > "/dev/stderr"
    exit 1
  }
  print
  next
}
{
  file = $1
  sub(/:.*/, "", file)
  if (file ~ /(^|\/)gen\// || file ~ /_templ\.go$/) next
  print
}
'
