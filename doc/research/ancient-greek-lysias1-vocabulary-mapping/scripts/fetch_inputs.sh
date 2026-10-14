#!/bin/sh
# Reproduce the pinned inputs for the Lysias I vocabulary-mapping research and verify them.
# Usage: fetch_inputs.sh <new-or-empty-dest-dir>
# Needs git, tar, curl, and sha256sum. Everything is fetched from public GitHub at pinned
# commits (no credentials). Nothing here is executed; it is read as data by derive_evidence.py.
#
# Layout written under DEST:
#   agdt/v2.1/Greek/texts/*.xml           all AGDT v2.1 Greek texts (Lysias I is verified by sha256)
#   sources/...                           AGDT tagsets, READMEs, guidelines, and the v1.6 / v2.0
#                                         Lysias I files (pre-normalization witness and 2.0 release)
#   canon/...                             canonical-greekLit TEI for Lysias I (verified by sha256)
#   ud/                                   UD_Ancient_Greek-Perseus at the pinned commit (cross-corpus evidence)
#   ext/<owner>/<repo>/<path>             pinned producer/converter/guideline files listed in EXT below
set -eu

DEST=$1
TB=bf4334f0af5e13d16b04c1cccd6237e683ac6f5f        # PerseusDL/treebank_data
CG=01b725d835e6e733062ffd79e0efdbae1ba06e5c        # PerseusDL/canonical-greekLit
UD=331ddef91411d0e6549744ee889e05549e6da77d        # UniversalDependencies/UD_Ancient_Greek-Perseus
CANON=data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
PDF11_BLOB=49972e555853abaf98d7ca61f049875b0c0a4779

mkdir -p "$DEST/agdt" "$DEST/sources" "$DEST/canon/data/tlg0540/tlg001" "$DEST/ext"

git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/treebank_data "$DEST/tb.git"
git clone --quiet --bare --filter=blob:none https://github.com/PerseusDL/canonical-greekLit "$DEST/cg.git"
git --git-dir="$DEST/tb.git" cat-file -e "$TB^{commit}"
git --git-dir="$DEST/cg.git" cat-file -e "$CG^{commit}"

git --git-dir="$DEST/tb.git" archive "$TB" v2.1/Greek/texts | tar -x -C "$DEST/agdt" -f -
git --git-dir="$DEST/tb.git" archive "$TB" v2.1/Greek/TAGSETS.xml v2.1/Greek/README.MD \
  AGDT2/guidelines/Greek_guidelines.md AGDT2/guidelines/Greek_guidelines_2.5.md \
  v1/greek/docs/guidelines.pdf v1/greek/docs/README.txt README.md v1.6/README.md \
  v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml \
  v2.0/Greek/README.MD v2.0/Greek/TAGSETS.xml \
  v2.0/Greek/nonArethusaCompliant/tlg0540.tlg001.perseus-grc1.tb.xml \
  AGDT2/corpus/readme.md AGDT2/corpus/stableannotations/readme.md | tar -x -C "$DEST/sources" -f -
# Which texts have second-annotator (stable, adjudicated) AGDT 2 annotations: names only.
git --git-dir="$DEST/tb.git" ls-tree -r --name-only "$TB" AGDT2/corpus/stableannotations > "$DEST/sources/agdt2-stableannotations.txt"
git --git-dir="$DEST/cg.git" show "$CG:$CANON" > "$DEST/canon/$CANON"

# UD is cross-corpus evidence only; the clone is pinned by commit.
git init --quiet "$DEST/ud"
git -C "$DEST/ud" fetch --quiet --depth 1 https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus "$UD"
git -C "$DEST/ud" checkout --quiet FETCH_HEAD

# Pinned producer/converter/guideline files: "owner/repo commit path".
EXT='perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 LICENSE
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/vbs.simp.ml
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/lsj.vbs
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/vbs.irreg
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/vbs.cmp.ml
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/vbs.cmp.lsj
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/nom.proper
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/stemsrc/lemlist
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 stemlib/Greek/steminds/entitylist.txt
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 src/anal/prntanal.c
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 src/anal/checkpreverb.c
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 src/anal/lemma_voice.c
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 src/morphlib/morphkeys.c
perseids-tools/morpheus ab6898ffed335fc6169fa02c9940657a9b5a78e0 src/gkdict/indexstems.c
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/AUX_.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/PART.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/ADV.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/PROPN.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/SCONJ.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/CCONJ.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/VERB.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _grc/pos/NOUN.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _u-pos/AUX_.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _u-pos/PART.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _u-pos/PROPN.md
UniversalDependencies/docs 24add7d0b041d11432f6694424111f1b7f50aa86 _u-dep/cop.md
UniversalDependencies/tools 77751a634188d0f9e2cfe104a24a65978f5bb1c4 data/data.json
ufal/treex df815f19c4d8251f13d99b396bdf32fc4f188762 lib/Treex/Block/HamleDT/HarmonizePerseus.pm
ufal/treex df815f19c4d8251f13d99b396bdf32fc4f188762 lib/Treex/Block/HamleDT/Udep.pm
ufal/treex df815f19c4d8251f13d99b396bdf32fc4f188762 lib/Treex/Block/HamleDT/GRC/Harmonize.pm
ufal/treex df815f19c4d8251f13d99b396bdf32fc4f188762 lib/Treex/Tool/PhraseBuilder/ToUD.pm
dan-zeman/interset f3568ffb0c3d73330df750fb57dd78d03ff1c0da lib/Lingua/Interset/Tagset/GRC/Conll.pm'
echo "$EXT" | while read -r repo sha path; do
  mkdir -p "$DEST/ext/$repo/$(dirname "$path")"
  curl -sSfL --retry 2 --max-time 120 "https://raw.githubusercontent.com/$repo/$sha/$path" -o "$DEST/ext/$repo/$path"
done

cd "$DEST"
sha256sum -c --quiet - <<'EOF'
44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568  agdt/v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml
5c8f187a489c67fa06bb70219f27177c8dd9b8061e198b9ee63a56f3d9bd9e27  sources/v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml
e3ad8b19166c6dffcc0e0b8a3d634b262fba0ef27a4b31329fa838ffaa78e467  sources/v2.0/Greek/nonArethusaCompliant/tlg0540.tlg001.perseus-grc1.tb.xml
21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a  canon/data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml
9b523bf62f866064658beae20ccd51e698b92068a5417f7d01742a8fa3ac38aa  ext/UniversalDependencies/docs/_grc/pos/ADV.md
c1bf37d2f6cfe14c088955fa6f621b7dd78733b9fac8ec50af67a12799d96af9  ext/UniversalDependencies/docs/_grc/pos/AUX_.md
6c172de127261ca6e83b8299ff687e0363c83378c20e3bf2a8e1fc8364555559  ext/UniversalDependencies/docs/_grc/pos/CCONJ.md
5179e649251ee2b12ef05a6c2ce5900d00cb05a7e910055aeecb10498d732936  ext/UniversalDependencies/docs/_grc/pos/NOUN.md
b5c3734b27ce017187360a43e31e08d54c51f67e8548f4b1fefa681a80361a2c  ext/UniversalDependencies/docs/_grc/pos/PART.md
354ecd5b2e76f2af281eab2bfb8b62dabc8801b8246e614cc9e2b52fb1f44d9c  ext/UniversalDependencies/docs/_grc/pos/PROPN.md
8ee7dd1873a03c017f09b2a76dea803fd8f6e35c7678c5141e594898cb334b48  ext/UniversalDependencies/docs/_grc/pos/SCONJ.md
a1aa87249f6208d22853c61a1baca802c2d3114a2aeb659c32b1e7b09427507b  ext/UniversalDependencies/docs/_grc/pos/VERB.md
c2c9508220f1e409e44274e2452034cf0b148a5597e806196c091482e94753ae  ext/UniversalDependencies/docs/_u-dep/cop.md
1435b177a68be843159910f6229877c5be4533ccedd1f07aa797d044d95cea7f  ext/UniversalDependencies/docs/_u-pos/AUX_.md
7eeebc05f7fa90369ee2674416568866bd5d3b4ff95d388b76c9e5895ee7d11b  ext/UniversalDependencies/docs/_u-pos/PART.md
a92d79170bdde2b99b86342dc7df75bf4c4e413d910db1dbc1ed6c344e61db2e  ext/UniversalDependencies/docs/_u-pos/PROPN.md
170358957edea42a1e29ff7bf3b408e1a5df30bcec866f3457f6d6cbf0a0ad26  ext/UniversalDependencies/tools/data/data.json
19abb2ee9b4fa8a8b4dfc1f7520b30b2aee143e4971177328e72fc2ec43cf976  ext/dan-zeman/interset/lib/Lingua/Interset/Tagset/GRC/Conll.pm
1f256ecad192880510e84ad60474eab7589218784b9a50bc7ceee34c2b91f1d5  ext/perseids-tools/morpheus/LICENSE
b8f84787687748453e237715a77f5da499ca808401184f4649fc88c7f1310699  ext/perseids-tools/morpheus/src/anal/checkpreverb.c
40018983f3609246e261e2a331864293a57f333d6ff4f77387d5de9805fb548a  ext/perseids-tools/morpheus/src/anal/lemma_voice.c
4429f08818c94e05c3d3eb9cf6b63c353d977b566ea8af51ac563eddac1ddcf1  ext/perseids-tools/morpheus/src/anal/prntanal.c
46777e70909c3e4dbaaa806ae078ccb3e96ec81d4f03f40109bfdc433fe8ae98  ext/perseids-tools/morpheus/src/gkdict/indexstems.c
0a986f3c18f0957be9ab8a2e833c301e34c088a610be6b2b5b6c0162e68946d8  ext/perseids-tools/morpheus/src/morphlib/morphkeys.c
cb4705256ef113a7a24cf500bda4ad7e31c36648ef2f34cb4661241a53989468  ext/perseids-tools/morpheus/stemlib/Greek/steminds/entitylist.txt
9c24975473c68111ff6b8a7e14020a221765ceaa854ffa499385e2a2904aec8d  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/lemlist
1169d2ccea85048e95742d12f58b49faf83fafc02f064abc2fa988239da6aa4c  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/lsj.vbs
2a122d2b42f749d52c9b76ff720443f9ad471ae9e79eb6c834a57660708f29ef  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/nom.proper
9d3ab2a6ed954fec63494dd69e571d525d5ea2e03f9a666406eb6052809770bc  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/vbs.cmp.lsj
e5e31787a96f77f07e56d9ebe04fe443b60ec9b66b49bbc920448c030eb89268  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/vbs.cmp.ml
05f20d682e502981fbfc09299d852edf066bf01be0d5624a74a58edbf7f043b1  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/vbs.irreg
857a00ce3cef756861f32af6380bab3537279eccb4cc2a0e3976438c202d7b5a  ext/perseids-tools/morpheus/stemlib/Greek/stemsrc/vbs.simp.ml
db9d0419598c01c3eac5adb7d0a4f82918cfeb4338dcd28946c73d5c09907c75  ext/ufal/treex/lib/Treex/Block/HamleDT/GRC/Harmonize.pm
1083bf98e3572d6fa100762e0f76449ed4e5a84f7c04093cbcad3da0002c2168  ext/ufal/treex/lib/Treex/Block/HamleDT/HarmonizePerseus.pm
5a92233f3ac0c2b8df897d997f407ee27aecddebac8894964be633949635061c  ext/ufal/treex/lib/Treex/Block/HamleDT/Udep.pm
d19cd52fc35c9db5dbb85641af0e14d0928c79d5499155a4f77bf63353de4896  ext/ufal/treex/lib/Treex/Tool/PhraseBuilder/ToUD.pm
EOF
test "$(git -C ud rev-parse HEAD)" = "$UD"
test "$(git --git-dir=tb.git rev-parse "$TB:v1/greek/docs/guidelines.pdf")" = "$PDF11_BLOB"
echo "inputs verified in $DEST"
