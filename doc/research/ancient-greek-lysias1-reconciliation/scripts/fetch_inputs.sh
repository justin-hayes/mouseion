#!/bin/sh
# Fetch every pinned input of the Lysias I reconciliation (#2048) and verify it by sha256.
# Usage: fetch_inputs.sh <new-or-empty-dest-dir>
# Needs git, tar and sha256sum. Everything comes from public GitHub at immutable commits.
# Set MOUSEION_REPO, TREEBANK_REPO or CANONICAL_REPO to a local clone to avoid re-downloading.
# Inputs are data: reconcile.py reads them with `python3 -I`; only the exported binding
# probe module from the pinned Mouseion commit is imported, for its canonical-text extractor.
set -eu

DEST=$1
HERE=$(cd "$(dirname "$0")" && pwd)
MOUSEION_REPO=${MOUSEION_REPO:-https://github.com/justin-hayes/mouseion}
TREEBANK_REPO=${TREEBANK_REPO:-https://github.com/PerseusDL/treebank_data}
CANONICAL_REPO=${CANONICAL_REPO:-https://github.com/PerseusDL/canonical-greekLit}

BINDING=dc3e5ed74681a24157d99316c25f2e18c141f748   # research/ancient-greek-lysias1-binding (#2048 r3)
MAPPING=50f4ad6ccb5df46daf603a14dc1d1393cc274c52   # research/ancient-greek-lysias1-vocabulary-mapping (#2049)
TB=bf4334f0af5e13d16b04c1cccd6237e683ac6f5f        # PerseusDL/treebank_data
CG=01b725d835e6e733062ffd79e0efdbae1ba06e5c        # PerseusDL/canonical-greekLit (pinned grc2)
CG_HIST=aa0dd9e6a25c848c04e75169e525ec413f033499   # canonical-greekLit, grc1 current on the annotation date

mkdir -p "$DEST/agdt" "$DEST/agdt-docs" "$DEST/canon" "$DEST/evidence/binding" "$DEST/evidence/mapping"

clone() {  # clone <url> <dir> <commit>
  [ -d "$2" ] || git clone --quiet --bare --filter=blob:none "$1" "$2"
  git --git-dir="$2" cat-file -e "$3^{commit}" 2>/dev/null || git --git-dir="$2" fetch --quiet origin "$3"
  git --git-dir="$2" cat-file -e "$3^{commit}"
}
clone "$MOUSEION_REPO" "$DEST/mouseion.git" "$BINDING"
clone "$MOUSEION_REPO" "$DEST/mouseion.git" "$MAPPING"
clone "$TREEBANK_REPO" "$DEST/tb.git" "$TB"
clone "$CANONICAL_REPO" "$DEST/cg.git" "$CG"
git --git-dir="$DEST/cg.git" cat-file -e "$CG_HIST^{commit}"

# Immutable evidence of the two reconciled investigations, exported by commit (not a worktree head).
git --git-dir="$DEST/mouseion.git" archive "$BINDING" doc/research/ancient-greek-lysias1-binding.md \
  doc/research/ancient-greek-lysias1-binding | tar -x -C "$DEST/evidence/binding" -f -
git --git-dir="$DEST/mouseion.git" archive "$MAPPING" doc/research/ancient-greek-lysias1-vocabulary-mapping \
  | tar -x -C "$DEST/evidence/mapping" -f -

LYS=tlg0540.tlg001.perseus-grc1.tb.xml
tb() { git --git-dir="$DEST/tb.git" show "$TB:$1" > "$2"; }
tb "v2.1/Greek/texts/$LYS" "$DEST/agdt/v2.1.xml"
tb "v2.0/Greek/nonArethusaCompliant/$LYS" "$DEST/agdt/v2.0.xml"
tb "v1.6/greek/data/$LYS" "$DEST/agdt/v1.6.xml"
tb README.md "$DEST/agdt-docs/root-README.md"
tb v1.6/README.md "$DEST/agdt-docs/v1.6-README.md"
tb v1/greek/docs/README.txt "$DEST/agdt-docs/v1-greek-docs-README.txt"
tb v2.0/Greek/README.MD "$DEST/agdt-docs/v2.0-Greek-README.MD"
tb v2.1/Greek/README.MD "$DEST/agdt-docs/v2.1-Greek-README.MD"

git --git-dir="$DEST/cg.git" show "$CG:data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml" > "$DEST/canon/grc2.xml"
git --git-dir="$DEST/cg.git" show "$CG:license.md" > "$DEST/canon/license.md"
git --git-dir="$DEST/cg.git" show "$CG_HIST:tlg0540/tlg001/tlg0540.tlg001.perseus-grc1.xml" > "$DEST/canon/grc1-aa0dd9e6.xml"
# The historical tree carries no licence file; record its root listing for the rights ledger.
git --git-dir="$DEST/cg.git" ls-tree --name-only "$CG_HIST" > "$DEST/canon/aa0dd9e6-root-listing.txt"

cd "$DEST"
sha256sum --quiet -c "$HERE/../inputs/sha256sums.txt"
echo "all pinned inputs verified"
