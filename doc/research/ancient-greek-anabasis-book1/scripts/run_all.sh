#!/usr/bin/env bash
# Reproduce every measurement in ../ancient-greek-anabasis-book1.md.
#
# Requirements: python3 (3.13 used), the pinned upstream files in $WORK/raw (see the report for
# gh api fetch commands and SHA256 values). Offline; no provider or network calls in this script.
# Outputs: $WORK/out/ (not committed; contains source-derived text) and the committed
# ../summaries/ directory (counts and short lists only).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
export WORK="${WORK:-$HOME/.cache/scratch/greek-2030-anabasis-validation}"
out="$WORK/out"
summ="$here/../summaries"
mkdir -p "$out" "$summ"
cd "$here"

python3 -I build_canonical.py > "$out/build_canonical.log"

for route in printed reading hybrid; do
  for flags in strict glyphok ambigignored glyphok_ambigignored; do
    env_prefix=()
    [[ "$flags" == *glyphok* ]] && env_prefix+=(GLYPH_OK=1)
    [[ "$flags" == *ambigignored* ]] && env_prefix+=(IGNORE_AMBIG=1)
    suffix=""; [[ "$flags" != strict ]] && suffix="_$flags"
    env "${env_prefix[@]}" python3 -I probe.py "$route" > /dev/null
    env "${env_prefix[@]}" python3 -I completeness.py "$route" "$suffix" > /dev/null
    cp "$out/summary_${route}${suffix}.json" "$out/completeness_${route}${suffix}.json" "$summ/"
  done
done
echo "done: summaries in $summ"
