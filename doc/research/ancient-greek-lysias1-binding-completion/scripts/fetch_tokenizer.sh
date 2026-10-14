#!/bin/sh
# Export the public llt-tokenizer sources the producer probe reads, at pinned commits (#2048 task A).
# Usage: fetch_tokenizer.sh <new-or-empty-dest-dir>
# Needs git and sha256sum. Only the two Ruby files that define the Greek enclitic/krasis path are exported.
set -eu

DEST=$1
HERE=$(cd "$(dirname "$0")" && pwd)
TOKENIZER_REPO=${TOKENIZER_REPO:-https://github.com/latin-language-toolkit/llt-tokenizer}

ANNOTATION_ERA=c9b60a345708af72c721ec1183c9ab928f7b8c03   # 2014-08-18, last commit before the v1.6 annotation date (2014-08-21)
POST_FIX=a4a810f02f73bacf3f16e722425dfb80f3b2a5ab          # 2014-08-25, changes the οὔτε accent in the enclitic map

mkdir -p "$DEST"
git clone --quiet --bare "$TOKENIZER_REPO" "$DEST/llt-tokenizer.git"
for c in "$ANNOTATION_ERA" "$POST_FIX"; do
  git --git-dir="$DEST/llt-tokenizer.git" cat-file -e "$c^{commit}"
  mkdir -p "$DEST/$c"
  git --git-dir="$DEST/llt-tokenizer.git" show "$c:lib/llt/tokenizer.rb" > "$DEST/$c/tokenizer.rb"
  git --git-dir="$DEST/llt-tokenizer.git" show "$c:lib/llt/tokenizer/greek.rb" > "$DEST/$c/greek.rb"
done

cd "$DEST"
sha256sum "$ANNOTATION_ERA/tokenizer.rb" "$ANNOTATION_ERA/greek.rb" "$POST_FIX/tokenizer.rb" "$POST_FIX/greek.rb" \
  > "$DEST/sha256sums.txt"
cat "$DEST/sha256sums.txt"
echo "tokenizer sources exported"
