#!/bin/sh
# Fetch the two public full-speech witnesses for #2048 revision 3 and the
# canonical-greekLit history blobs used by lineage_collation.py.
# Usage: fetch_witnesses.sh <inputs-dir-from-fetch_inputs.sh>
# Needs curl, git and sha256sum. No credentials, cookies are not kept.
# Witness files are untrusted data: they are only read via `python3 -I`.
set -eu

DEST=$1
mkdir -p "$DEST/history" "$DEST/witness/mnemosyne" "$DEST/witness/wikisource"

# 1. canonical-greekLit history: every distinct blob of the Lysias I Greek file.
OLD=tlg0540/tlg001/tlg0540.tlg001.perseus-grc1.xml
G1=data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc1.xml
G2=data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
while read -r c p; do
  git --git-dir="$DEST/cg.git" show "$c:$p" > "$DEST/history/$c.xml"
done <<EOF
dc36cfe1fae36acd800418baf16e8a2c403749fa $OLD
aa0dd9e6a25c848c04e75169e525ec413f033499 $OLD
365e93f4543788b25f25013f2dc1932151a7e210 $G1
b9d99e44a9552cc0ba12c785b4a61cbbdb09a6f0 $G1
8d9937ed69e5af4edecde098fab680a5c0045468 $G1
0a3eab7fccf72ca099241662cd1eda7737086779 $G1
8177550ef85696f54327ed9ca310b9661e31a932 $G1
16d2842d68bb588577164e8a28dfa006317a6ac5 $G1
7a61daad6fad68ef2f74058419ef62de0312312b $G1
82af5ab7086d198c7994b9f6dc43345149d96f54 $G2
0b7bef19fa1f759d1702e20280e35f37ba787ace $G2
b01744e86530350db8f81a019072e68880e470fd $G2
50c80234c89c8977531f09a3136c079dc24eeef9 $G2
5d9443283069e004e989b8e611561ff02c86fa12 $G2
0c75c6009460b05198e1050d67e48a16966acc72 $G2
feb8f32e56399f91e5c98ccb0a6aaab5c062879e $G2
dd96fe31bc5150a84a0f7259518abbcaf12a2454 $G2
70427323b61267cdcd0256b9b02009c23392de07 $G2
EOF

# 2. Centre for the Greek Language, Mnemosyne digital library, text_id=38 (7 pages).
#    Public web pages, (c) 2012 Kentro Ellinikis Glossas, all rights reserved: fetched
#    for private collation only; their text is not committed.
for p in 1 2 3 4 5 6 7; do
  curl -sS --fail -o "$DEST/witness/mnemosyne/page$p.html" \
    "https://www.greek-language.gr/digitalResources/ancient_greek/library/browse.html?text_id=38&page=$p"
  sleep 1
done
curl -sS --fail -o "$DEST/witness/mnemosyne/contributors.html" \
  "https://www.greek-language.gr/digitalResources/ancient_greek/library/contributors.html"

# 3. Greek Wikisource, page id 7431, pinned revisions (wikitext via action=raw).
WS='https://el.wikisource.org/w/index.php'
for r in 19883 20458 29293 48054 86278 91877 102844; do
  curl -sS --fail -G "$WS" -d action=raw -d "oldid=$r" -o "$DEST/witness/wikisource/rev$r.txt"
  sleep 1
done

cd "$DEST"
sha256sum -c - <<'EOF'
0c2c517bb98960b75d4f1bc7c04c55e02b019d2fe9e01165f0064a8bad700a43  witness/mnemosyne/page1.html
253f4ebe80a0bc9fdc1230fc53095936319aff68c76fbf53e61b9a6dff7a5710  witness/mnemosyne/page2.html
fe8208b4b66615d1057567e9ed1aa36efb2bf8bae28dfc225945a838fb08886d  witness/mnemosyne/page3.html
3889704c7eb876a080e915828d68e84d15349e5f61ca2954fe628a1bc84052f6  witness/mnemosyne/page4.html
2322afbdd5a4cdfff331d547987030c74fd499522ef76632dfc4ed75755fee75  witness/mnemosyne/page5.html
845acbd3d7b403b17a47a86b732ccb74860c6d4ab4fe9e0c5a892ab94451c51e  witness/mnemosyne/page6.html
3f91adea58bd994431050c5de82820be7beb28a582816b7fccfd6e5ad187cde2  witness/mnemosyne/page7.html
761e4cbad018575cddc36943b68b74bbb004278e7b8e2bef13a038387caf6672  witness/mnemosyne/contributors.html
25fa4bd2659bfe883a3b6626ffd14d6087d0b3b0d1301f1a4538359468d8846a  witness/wikisource/rev19883.txt
e1089ecdc7663d752453c8f3a245ed83f7f2079791e9d03b05ccf0e66d34f5ee  witness/wikisource/rev48054.txt
cfac0323802c342120ec23ddf473e926caea7d225d83dd9aa94c93a5f02dc373  witness/wikisource/rev86278.txt
d0606d69b6af003ce8e99ef2d2d46dc70492af371425d487c445c12409fb44ad  witness/wikisource/rev102844.txt
cd00710eff2d7bc4775ef789d8d32c3c7e85e843d7f0c455f6f11f931e24ab5a  witness/wikisource/rev20458.txt
a3cf34bef04ed478cf59721b919dca9fe9cb70bf8a1496e10b6a14e7c605b7eb  witness/wikisource/rev29293.txt
1f29609979fd2ebb2f4a1834b1b879aabc53e776dab6dc93ed9091b593740c8d  witness/wikisource/rev91877.txt
EOF
