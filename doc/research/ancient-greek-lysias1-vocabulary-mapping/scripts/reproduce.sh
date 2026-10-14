#!/bin/sh
# Fetch and hash-verify the pinned inputs, regenerate the manifest, and run the tests.
# Usage: reproduce.sh <new-or-empty-scratch-dir>
# Example: reproduce.sh "$HOME/.cache/scratch/lysias1-repro"
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
DEST=${1:?usage: reproduce.sh <scratch-dir>}

sh "$HERE/fetch_inputs.sh" "$DEST"
LYSIAS1_INPUTS="$DEST" python3 -I "$HERE/test_mapping.py"
