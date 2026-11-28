#!/usr/bin/env bash
# Compares a deadcode report with the committed allowlist.
#
# Usage: tools/deadcode-check.sh ALLOWLIST < report
#
# The report has one "<import path>.<function> <file>" line per unreachable
# function (the template `make deadcode` passes to deadcode -f). Reports in gen/
# are ignored: protobuf and gRPC output nobody writes by hand. Fails on a report
# the allowlist does not name, on an allowlist entry that matches no report, and
# on an entry without a reason.
set -euo pipefail

allowlist=${1:?usage: deadcode-check.sh ALLOWLIST < report}

awk -v allowlist="$allowlist" '
  BEGIN {
    while ((getline line < allowlist) > 0) {
      if (line ~ /^[[:space:]]*(#|$)/) continue
      sub(/^[[:space:]]+/, "", line)
      name = line; sub(/[[:space:]].*$/, "", name)
      reason = line; sub(/^[^[:space:]]+[[:space:]]*/, "", reason)
      if (reason == "") {
        printf "allowlist entry has no reason: %s\n", name > "/dev/stderr"; bad = 1
      }
      allowed[name] = 1
    }
  }
  $2 ~ /^gen\// { next }
  NF >= 1 {
    seen[$1] = 1
    if (!($1 in allowed)) {
      printf "unreachable from any main package: %s (%s)\n", $1, $2 > "/dev/stderr"; bad = 1
    }
  }
  END {
    for (name in allowed) if (!(name in seen)) {
      printf "stale allowlist entry (no longer reported): %s\n", name > "/dev/stderr"; bad = 1
    }
    exit bad
  }
'
