# Source manifest

Inputs were fetched into a scratch directory outside the repo. Nothing from the
sources is committed. Each row gives the URL, the pin, and the SHA-256 of the exact
bytes measured. Re-fetch and compare before reusing a result.

## Pinned inputs

| Input | Fetch URL | Pin | Bytes | SHA-256 |
|---|---|---|---|---|
| OGA `oga_src` v0.2.0 zip | `https://zenodo.org/api/records/15199546/files/mgbilby/oga_src-v0.2.0.zip/content` | Zenodo DOI 10.5281/zenodo.15199546 | 761,489,280 | `152e26c48166bc9fea6062ee789a93e70a388c8dc3fcd2dafc8a2c61189ab35a` (MD5 `714b9b52abc47c65f26f6f2fefc92520`, matches Zenodo) |
| OGA pinned member `tlg0032.tlg006.perseus-grc2_oga020_src.conllu` | inside the zip at `mgbilby-oga_src-223d225/data/` | same zip | 7,438,037 | `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61` (matches `internal/ancientgreek/ancientgreek.go:23-39`) |
| PerseusDL/lexica zip | `https://codeload.github.com/PerseusDL/lexica/zip/56061ca127f4a2844980baffc5f2b6d1332897b3` | commit `56061ca127f4a2844980baffc5f2b6d1332897b3` (committer date 2026-09-02) | 61,883,843 | `8d5bb101f912a9b0a8e09ecaa622b6a31d0dc895803a36e926ddcb63d0dfa910` |
| Middle Liddell TEI (fork copy) | `https://raw.githubusercontent.com/blinskey/middle-liddell/5db8d88a20066e182ca3fc6ee758c9f61ac60603/Perseus_text_1999.04.0058.xml` | commit `5db8d88a20066e182ca3fc6ee758c9f61ac60603` | 23,266,984 | `1b0e2ed672ac23399fbdb031b06119e7736ccefc78c229f6c9302b7c6348c811` |
| Kaikki raw Wiktextract, all languages (gzip) | `https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz` | Last-Modified 2026-10-03; the page says extracted 2026-10-03 from the enwiktionary dump dated 2026-09-02 | 2,981,058,381 | `3dac8a09e57827bef493e2e6b552fe917bf3b1fc55dee05c4e4a3702aff5dbdb` |
| Kaikki `grc` subset, derived | `scripts/filter_kaikki_grc.py` applied to the gzip above | derived; 68,196 top-level `grc` lines | 389,780,362 | `65c795d9ca6e1df91dfbdbe288697d576afa7c289a361512431c8a0c24727a12` |

### Kaikki caveats

- The derived subset has no immutable upstream URL. Its pin is the gzip hash plus the
  filter script. The filter is deterministic, but the subset was derived once. The
  single derivation is what every number in this file uses.
- A separate local file `x/kaikki-grc/grc.jsonl` (SHA-256 `38a02793…`) has a different
  hash from the derived subset above. Its provenance is not recorded and it was not used
  for any figure. Do not mix the two.
- Upstream identity, as the raw-data page states: "extracted on 2026-10-03 from the
  enwiktionary dump dated 2026-09-02 using wiktextract (1a05e46 and e3d6d4e)." The commit
  `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a` resolves in `tatuylonen/wiktextract`
  (committer date 2026-09-21). `e3d6d4e` did not resolve through the GitHub API (HTTP
  422), so it is recorded as stated by the page and not verified.
- The per-language postprocessed file `kaikki.org-dictionary-AncientGreek.jsonl`
  (SHA-256 `69d7185819de51cc746d96f8033eb6c3f3e2016a314c2ee640ec09ce80623029`, 403,924,438
  bytes) is marked DEPRECATED on the raw-data page. The first draft measured it. Both
  files give 68,196 top-level `grc` entries.

## Per-file and licence sources

| Source | Path or URL at the pin | SHA-256 |
|---|---|---|
| Lexica README (lexica-56061ca1) | `README.md` | `f75ece374961e57c312c9ff12f13606f9f65ad20007378e1086ffc9b2110b5d2` |
| Lexica `license.md` (CC BY-SA 4.0 full text) | `license.md` | `ccf0e8ce183761bf82700126cc45a4e907b2cede024e65e3fef5c2338b9b7063` |
| LSJ files (27 TEI P4 XML, `grc.lsj.perseus-eng1.xml` to `eng27`) and LSJ README | `CTS_XML_TEI/perseus/pdllex/grc/lsj/` | listed in `summaries/hashes-perseus-lsj.txt` |
| Per-source hashes | — | `summaries/hashes-sources.txt` |

The zip contains only LSJ and Lewis & Short (Latin) under `grc`/`lat`. Middle Liddell is
not in it: a full-zip search (44 entries, all paths and text headers) found no
`1999.04.0058` file and no "Intermediate Greek-English" header. The Middle Liddell TEI
used here is the fork copy above.

Web pages used for print-edition and licence statements (Perseus hopper pages, DGE,
Bailly 2020, WMF Terms of Use, wiktextract LICENSE, Wikipedia and HathiTrust authority
pages) are cited by URL in the research file. They were fetched on 2026-10-17. Some were
reachable only over HTTP.

## Crosswalk-tier output

`summaries/crosswalk-tiers.json` is produced by `scripts/crosswalk_tiers.py` from the
Anabasis file and the three dictionaries. Its identity count (1,429) equals
`summaries/book1-lookup-measurement.json`. Tier counts sum to 1,429 per source.

## Repro

Run from any directory. Use a scratch path, not `/tmp`.

```sh
S=$HOME/.cache/scratch/greek-lexica-2133   # scratch, not /tmp
R=/path/to/repo/doc/research/ancient-greek-lexica
mkdir -p $S/dl/{kaikki-raw,perseus,ml,oga} $S/x && cd $S

curl -sSL -o dl/kaikki-raw/raw-wiktextract-data.jsonl.gz 'https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz'
zcat dl/kaikki-raw/raw-wiktextract-data.jsonl.gz | python3 -I $R/scripts/filter_kaikki_grc.py > x/grc-top.jsonl
curl -sSL -o dl/perseus/lexica.zip 'https://codeload.github.com/PerseusDL/lexica/zip/56061ca127f4a2844980baffc5f2b6d1332897b3'
curl -sSL -o dl/ml/ml-blinskey.xml 'https://raw.githubusercontent.com/blinskey/middle-liddell/5db8d88a20066e182ca3fc6ee758c9f61ac60603/Perseus_text_1999.04.0058.xml'
curl -sSL -o dl/oga/oga_src-v0.2.0.zip 'https://zenodo.org/api/records/15199546/files/mgbilby/oga_src-v0.2.0.zip/content'

sha256sum dl/kaikki-raw/*.gz dl/perseus/lexica.zip dl/ml/*.xml dl/oga/*.zip x/grc-top.jsonl   # compare with the tables above

python3 -I -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" dl/perseus/lexica.zip x/perseus
python3 -I -c "
import zipfile,sys
z=zipfile.ZipFile(sys.argv[1])
z.extract('mgbilby-oga_src-223d225/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu', sys.argv[2])
" dl/oga/oga_src-v0.2.0.zip x/oga

LSJ=x/perseus/lexica-56061ca127f4a2844980baffc5f2b6d1332897b3/CTS_XML_TEI/perseus/pdllex/grc/lsj
CONLLU=x/oga/mgbilby-oga_src-223d225/data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu

python3 -I -B $R/scripts/measure.py $CONLLU x/grc-top.jsonl $LSJ dl/ml/ml-blinskey.xml summaries-out.json
python3 -I -B $R/scripts/crosswalk_tiers.py $CONLLU x/grc-top.jsonl $LSJ dl/ml/ml-blinskey.xml crosswalk-out.json
```

`measure.py` and `crosswalk_tiers.py` find `betacode.py` from their own directory
(`os.path.abspath(__file__)`), so `-I` works from any working directory with an
absolute script path. Diff `summaries-out.json` against
`summaries/book1-lookup-measurement.json`. A rerun in this pass produced identical JSON.
