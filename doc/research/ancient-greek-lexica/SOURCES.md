# Source manifest

Inputs were fetched into a scratch directory outside the repo. Nothing from the
sources is committed. Hashes are from the run that produced
`summaries/book1-lookup-measurement.json`. Re-fetch and compare before reusing.

| Source | URL | Pin | SHA-256 |
|---|---|---|---|
| Kaikki grc JSONL | `https://kaikki.org/dictionary/Ancient%20Greek/kaikki.org-dictionary-AncientGreek.jsonl` | last-modified 2026-10-03; not an upstream commit | `69d7185819de51cc746d96f8033eb6c3f3e2016a314c2ee640ec09ce80623029` |
| PerseusDL/lexica zip | `https://codeload.github.com/PerseusDL/lexica/zip/56061ca127f4a2844980baffc5f2b6d1332897b3` | commit `56061ca127f4a2844980baffc5f2b6d1332897b3` | `8d5bb101f912a9b0a8e09ecaa622b6a31d0dc895803a36e926ddcb63d0dfa910` |
| Middle Liddell (fork) | `https://raw.githubusercontent.com/blinskey/middle-liddell/5db8d88a20066e182ca3fc6ee758c9f61ac60603/Perseus_text_1999.04.0058.xml` | commit `5db8d88a20066e182ca3fc6ee758c9f61ac60603` | `1b0e2ed672ac23399fbdb031b06119e7736ccefc78c229f6c9302b7c6348c811` |
| Middle Liddell (unlicensed copy) | `https://raw.githubusercontent.com/adiel-mittmann/middle-liddell/b492bdf2289aa3bf4d61c597e2c215201cbca9b8/middle-liddell.xml` | commit `b492bdf2289aa3bf4d61c597e2c215201cbca9b8` | `70bef324ee4087a96f201aed97740ed04440b543b3e178c54047bf75366be351` |
| OGA oga_src v0.2.0 zip | `https://zenodo.org/api/records/15199546/files/mgbilby/oga_src-v0.2.0.zip/content` | Zenodo 15199546 v0.2.0; MD5 `714b9b52abc47c65f26f6f2fefc92520` (Zenodo) | `152e26c48166bc9fea6062ee789a93e70a388c8dc3fcd2dafc8a2c61189ab35a` |
| OGA pinned member | `mgbilby-oga_src-223d225/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu` inside the zip | 7,438,037 bytes | `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61` (matches `internal/ancientgreek/ancientgreek.go`) |

The per-file hashes for the LSJ XML and the Perseus README and licence are in
`summaries/hashes-perseus-lsj.txt`. The top-level hashes are in
`summaries/hashes-sources.txt`.

Not fetched in this pass: Morpheus (`PerseusDL/morpheus@b1b33c56`, from the
earlier proof), UD Ancient Greek-Perseus, the Gorman treebank, DGE, Bailly, and Pape.

## Repro

```sh
S=$HOME/.cache/scratch/greek-lexica-2133   # scratch, not /tmp
mkdir -p $S/dl/{kaikki,perseus,ml,oga} && cd $S
curl -sSL -o dl/kaikki/kaikki-AncientGreek.jsonl 'https://kaikki.org/dictionary/Ancient%20Greek/kaikki.org-dictionary-AncientGreek.jsonl'
curl -sSL -o dl/perseus/lexica-56061ca1.zip 'https://codeload.github.com/PerseusDL/lexica/zip/56061ca127f4a2844980baffc5f2b6d1332897b3'
curl -sSL -o dl/ml/ml-blinskey-5db8d88a.xml 'https://raw.githubusercontent.com/blinskey/middle-liddell/5db8d88a20066e182ca3fc6ee758c9f61ac60603/Perseus_text_1999.04.0058.xml'
curl -sSL -o dl/oga/oga_src-v0.2.0.zip 'https://zenodo.org/api/records/15199546/files/mgbilby/oga_src-v0.2.0.zip/content'
sha256sum dl/kaikki/*.jsonl dl/perseus/*.zip dl/ml/*.xml dl/oga/*.zip   # compare with the table
python3 -I -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" dl/perseus/lexica-56061ca1.zip dl/perseus/x
python3 -I -c "import zipfile,sys; z=zipfile.ZipFile(sys.argv[1]); z.extract('mgbilby-oga_src-223d225/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu', sys.argv[2])" dl/oga/oga_src-v0.2.0.zip dl/oga/x
python3 -I /path/to/repo/doc/research/ancient-greek-lexica/scripts/measure.py \
  dl/oga/x/mgbilby-oga_src-223d225/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu \
  dl/kaikki/kaikki-AncientGreek.jsonl \
  dl/perseus/x/lexica-56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj \
  dl/ml/ml-blinskey-5db8d88a.xml out.json
```

`measure.py` imports `betacode.py` from its own directory, so run it with `-I`
from the scripts directory or with that directory on the path. The Middle Liddell
file path must be the fork XML, not the Kaikki or LSJ directory.
