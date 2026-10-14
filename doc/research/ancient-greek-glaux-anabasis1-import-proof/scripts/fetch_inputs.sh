#!/usr/bin/env bash
# Fetch every pinned input listed in inputs/pinned-inputs.tsv into DIR and
# verify its sha256. Files already present with the right hash are kept.
# Usage: scripts/fetch_inputs.sh DIR
set -euo pipefail

dir=${1:?usage: fetch_inputs.sh DIR}
here=$(cd "$(dirname "$0")/.." && pwd)
registry="$here/inputs/pinned-inputs.tsv"

mkdir -p "$dir"
status=0
while IFS=$'\t' read -r path _role url _revision sha _licence; do
  [ "$path" = path ] && continue
  target="$dir/$path"
  if [ -f "$target" ] && [ "$(sha256sum "$target" | cut -d' ' -f1)" = "$sha" ]; then
    continue
  fi
  mkdir -p "$(dirname "$target")"
  curl -sSfL --retry 3 -o "$target.part" "$url"
  mv "$target.part" "$target"
  got=$(sha256sum "$target" | cut -d' ' -f1)
  if [ "$got" != "$sha" ]; then
    echo "hash mismatch: $path expected $sha got $got" >&2
    status=1
  fi
done < "$registry"
exit $status
