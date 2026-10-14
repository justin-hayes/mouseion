#!/usr/bin/env bash
# One command for the whole proof (#2050): fetch and verify pinned inputs,
# import Book 1, run Mouseion's Go selection code on the result, check Go and
# Python selection parity, compare every committed manifest byte for byte,
# and run the tests.
#
# Usage: scripts/reproduce.sh [INPUT_DIR] [--update]
#   INPUT_DIR defaults to ~/.cache/scratch/greek-glaux-anabasis1-import-proof/inputs
#   --update rewrites manifest/ instead of comparing against it.
set -euo pipefail

here=$(cd "$(dirname "$0")/.." && pwd)
repo=$(cd "$here/../../.." && pwd)
inputs=${1:-$HOME/.cache/scratch/greek-glaux-anabasis1-import-proof/inputs}
update=${2:-}
work=$(mktemp -d "$HOME/.cache/scratch/glaux-proof-run.XXXXXX")
trap 'rm -rf "$work"' EXIT
export GOTMPDIR=${GOTMPDIR:-$HOME/.cache/gotmp}
mkdir -p "$GOTMPDIR"

"$here/scripts/fetch_inputs.sh" "$inputs"
python3 -I "$here/scripts/import_proof.py" "$inputs" "$work/out" >/dev/null
(cd "$repo" && go run ./doc/research/ancient-greek-glaux-anabasis1-import-proof/_selectionprobe "$work/out/analysis.json" "$work/out/selection_go.tsv")
cmp "$work/out/selection_go.tsv" "$work/out/selection_model.tsv"
echo "go/python selection parity: identical"
rm "$work/out/analysis.json"

if [ "$update" = --update ]; then
  rm -rf "$here/manifest"
  cp -r "$work/out" "$here/manifest"
  echo "manifest/ updated"
else
  diff -r "$here/manifest" "$work/out"
  echo "manifest/: byte-identical regeneration"
fi

cd "$here"
GLAUX_PROOF_INPUTS=$inputs python3 -I -m unittest discover -s scripts -p 'test_*.py'
