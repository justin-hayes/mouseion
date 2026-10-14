#!/bin/sh
# Reproduce the pinned inputs and verify their hashes.
# Usage: fetch_inputs.sh <empty-or-new-dest-dir>
# Needs git and sha256sum. Blobs are fetched on demand from GitHub (no credentials).
set -eu

DEST=$1
TB=bf4334f0af5e13d16b04c1cccd6237e683ac6f5f
CG=01b725d835e6e733062ffd79e0efdbae1ba06e5c
AGDT_PATH=v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml
CANON_PATH=data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml

mkdir -p "$DEST/agdt/v2.1/Greek/texts" "$DEST/canon/data/tlg0540/tlg001"

git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/treebank_data "$DEST/tb.git"
git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/canonical-greekLit "$DEST/cg.git"
git --git-dir="$DEST/tb.git" cat-file -e "$TB^{commit}"
git --git-dir="$DEST/cg.git" cat-file -e "$CG^{commit}"

git --git-dir="$DEST/tb.git" show "$TB:$AGDT_PATH" > "$DEST/agdt/$AGDT_PATH"
git --git-dir="$DEST/cg.git" show "$CG:$CANON_PATH" > "$DEST/canon/$CANON_PATH"

cd "$DEST"
sha256sum -c - <<'EOF'
44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568  agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml
21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a  canon/data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
EOF
