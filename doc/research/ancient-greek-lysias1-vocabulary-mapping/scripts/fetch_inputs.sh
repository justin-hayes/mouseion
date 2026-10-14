#!/bin/sh
# Reproduce the pinned inputs for the Lysias I vocabulary-mapping research and verify them.
# Usage: fetch_inputs.sh <new-or-empty-dest-dir>
# Needs git, tar, and sha256sum. Blobs are fetched on demand from GitHub (no credentials).
#
# Layout written under DEST:
#   agdt/v2.1/Greek/texts/*.xml          all AGDT v2.1 Greek texts (Lysias I is verified by sha256)
#   sources/v2.1/Greek/{TAGSETS.xml,README.MD}
#   sources/AGDT2/guidelines/Greek_guidelines{,_2.5}.md   (AGDT 2.0 and 2.5 guidelines)
#   sources/v1/greek/docs/guidelines.pdf  (AGDT 1.1 guidelines; the scheme Lysias I declares)
#   canon/...                            canonical-greekLit TEI for Lysias I (verified by sha256)
#   ud/                                  UD_Ancient_Greek-Perseus at the pinned commit (cross-corpus evidence)
set -eu

DEST=$1
TB=bf4334f0af5e13d16b04c1cccd6237e683ac6f5f        # PerseusDL/treebank_data
CG=01b725d835e6e733062ffd79e0efdbae1ba06e5c        # PerseusDL/canonical-greekLit
UD=331ddef91411d0e6549744ee889e05549e6da77d        # UniversalDependencies/UD_Ancient_Greek-Perseus
CANON=data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
PDF11_BLOB=49972e555853abaf98d7ca61f049875b0c0a4779

mkdir -p "$DEST/agdt" "$DEST/sources" "$DEST/canon/data/tlg0540/tlg001"

git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/treebank_data "$DEST/tb.git"
git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/canonical-greekLit "$DEST/cg.git"
git --git-dir="$DEST/tb.git" cat-file -e "$TB^{commit}"
git --git-dir="$DEST/cg.git" cat-file -e "$CG^{commit}"

git --git-dir="$DEST/tb.git" archive "$TB" v2.1/Greek/texts | tar -x -C "$DEST/agdt" -f -
git --git-dir="$DEST/tb.git" archive "$TB" v2.1/Greek/TAGSETS.xml v2.1/Greek/README.MD \
  AGDT2/guidelines/Greek_guidelines.md AGDT2/guidelines/Greek_guidelines_2.5.md \
  v1/greek/docs/guidelines.pdf | tar -x -C "$DEST/sources" -f -
git --git-dir="$DEST/cg.git" show "$CG:$CANON" > "$DEST/canon/$CANON"

# UD is cross-corpus evidence only; the clone is pinned by commit.
git init --quiet "$DEST/ud"
git -C "$DEST/ud" fetch --quiet --depth 1 https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus "$UD"
git -C "$DEST/ud" checkout --quiet FETCH_HEAD

cd "$DEST"
sha256sum -c - <<'EOF'
44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568  agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml
21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a  canon/data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
EOF
test "$(git -C ud rev-parse HEAD)" = "$UD"
test "$(git --git-dir=tb.git rev-parse "$TB:v1/greek/docs/guidelines.pdf")" = "$PDF11_BLOB"
echo "inputs verified in $DEST"
