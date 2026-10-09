#!/usr/bin/env bash
# Decides the required "Browser smoke" check from upstream job results.
#
# Usage: tools/ci-gate.sh <changes-result> <browser-required> <shards-result>
#
# The check passes only when change detection succeeded and the browser shards
# ended in the state the classification asked for: success when browser
# checks are required, skipped when they are not. A failed, cancelled, or
# unexpectedly skipped shard therefore never reports success.
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <changes-result> <browser-required> <shards-result>" >&2
  exit 2
fi

changes_result=$1
browser_required=$2
shards_result=$3

if [ "$changes_result" != success ]; then
  echo "change detection ended with '$changes_result'; browser smoke cannot be decided" >&2
  exit 1
fi

if [ "$browser_required" = true ]; then
  expected=success
else
  expected=skipped
fi

if [ "$shards_result" != "$expected" ]; then
  echo "browser shards ended with '$shards_result'; expected '$expected'" >&2
  exit 1
fi

echo "browser smoke: shards '$shards_result' as expected (browser required: $browser_required)"
