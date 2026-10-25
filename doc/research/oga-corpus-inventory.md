# Opera Graeca Adnotata / oga_src: which Ancient Greek works the releases provide, and in what shape

**Research date:** 2026-10-25

**Question** ([#2293](https://github.com/justin-hayes/mouseion/issues/2293), map
[#2292](https://github.com/justin-hayes/mouseion/issues/2292)): which works and
editions do the `oga_src` releases (0.2.0, and any later release) and the
underlying OGA corpus provide, and can a shelf of Reading units be derived from
them mechanically?

**Companion file:** [`oga-corpus-inventory.tsv`](oga-corpus-inventory.tsv) is the
per-file inventory (2,000 rows: path, CTS URN, edition tag, author, title, token
and sentence counts, byte size, SHA-256 of the file at tag `v0.2.0`). It was
produced by streaming every file once from the tag and running the checks below.

**Related research:** [per-work OGA packaging](https://github.com/justin-hayes/mouseion/blob/research/ancient-greek-oga-per-work-packaging/doc/research/ancient-greek-oga-per-work-packaging.md)
(branch `research/ancient-greek-oga-per-work-packaging`, not yet on `main`)
covers redistribution rights and the Anabasis package; this note does not
repeat that work.

## Verdict

- **The release is one annotation snapshot of 2,000 files, not a set of works
  with a shelf structure.** `oga_src` 0.2.0 contains 1,999 distinct CTS works
  (1,811 TLG works, 182 Patristic Text Archive works, plus 6 others) and 2,000
  files, because `tlg5026.tlg001` is split into two files. Total: 40,103,489
  token lines and 2,234,754 sentences.
- **Each work is one individually addressable file** at a stable raw URL and
  has a size and SHA-256 that can be pinned. The Zenodo deposit is a single
  761 MB zip, so pinning must use the GitHub tag or commit, not the Zenodo
  zip.
- **Citations are recoverable mechanically** from the per-token `ref=` in the
  MISC field for 1,999 of 2,000 files. Only `tlg0007.tlg082` (Plutarch,
  *Apophthegmata Laconica*, `perseus-grc3`) lacks token refs; its
  sentence-level source headers still carry a location.
- **The files are not one dialect.** Six files repeat the `# source` header
  line in every sentence, one file has a stray `J` before its first `# source`,
  and 26 files use Greek-letter relation labels. The current strict adapter
  (`internal/ancientgreek/ogasrc.go`) would refuse the first two classes, and
  it treats POS `x` as an ellipsis, which is wrong for the 1,186 files where
  `x` marks real tokens. Details under [Dialect](#dialect-uniformity).
- **Provenance is automatic.** The OGA paper describes morphosyntax as the
  output of the COMBO parser trained on AGDT 2.1; tokenization, sentence
  segmentation and CTS citation are rule-based. Nothing in the sources read
  here documents a manual correction pass for the 0.2.0 files.
- **Licence:** CC BY-SA 4.0 per `oga_src` `license.md`, matching the upstream
  Perseus and First1KGreek repositories. The Zenodo metadata for the OGA corpus
  record reads `other-at`, which is not a CC identifier; see
  [Licence](#licence-and-provenance).

## Sources checked

| Source | What it gave |
| --- | --- |
| Zenodo record 15199546 (`oga_src` v0.2.0, DOI 10.5281/zenodo.15199546), `api/records/15199546` | Title, publication date 2025-04-11, licence `cc-by-sa-4.0`, single file `mgbilby/oga_src-v0.2.0.zip` (761,489,280 bytes, md5 `714b9b52abc47c65f26f6f2fefc92520`), description (file layout, header and MISC format, Perseus/First1KGreek lineage) |
| Zenodo `api/records/15199546/versions` and search for the concept 15199545 | One version only (v0.2.0) |
| Zenodo record 14206061 (OGA corpus v0.2.0, DOI 10.5281/zenodo.14206061) | Single zip `opera_graeca_adnotata_v0.2.0.zip`, 8,629,032,478 bytes, md5 `d4117ae52f1cc6319c6ba20af0c1cf3b`; licence field `other-at`; versions 0.2.0 (2024-11-24) and 0.1.0 (Zenodo 8158675, 2023-07-18) |
| GitHub `mgbilby/oga_src`: tags, releases, git trees (`tag v0.2.0`, `main`), `license.md`, `README.md` | Tag `v0.2.0` at commit `223d2258745ecf35e7659191080f93640fe900c1`; release `v0.2.0`; 2,000 `.conllu` blobs under `data/`; `main` has the same 2,000 blobs as the tag (blob SHA identical for every file); last push 2026-10-16 (`funding.yml`) |
| Celano, *Opera Graeca Adnotata: Building a 34M+ Token Multilayer Corpus for Ancient Greek*, arXiv:2404.00739 (describes OGA 0.1.0) | Morphosyntax by COMBO trained on AGDT 2.1; rule-based tokenization, sentence segmentation and CTS citation; source corpora Perseus (canonical-greekLit) and First1KGreek |
| Perseus canonical-greekLit `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml` | Edition statement: Marchant, Clarendon Press 1904 (reprint 1961); licence CC BY-SA 4.0 in `<availability>`; CTS URN `urn:cts:greekLit:tlg0032.tlg006.perseus-grc2` |
| canonical-greekLit and First1KGreek repository licence (GitHub API) | Both `CC-BY-SA-4.0` |
| All 2,000 files, streamed from the tag (`raw.githubusercontent.com/mgbilby/oga_src/v0.2.0/data/…`) | Size, SHA-256 and dialect checks (see [Method](#method)) |

Not checked: the Patristic Text Archive repository licence (the GitHub API
path I tried returned Not Found), the Open Greek Patristic (`opp`) and other
minor edition sources, and the OGA 0.2.0 corpus README (only available inside
the 8.6 GB zip, which I did not download).

## Packaging and addressability

- **`oga_src` 0.2.0 on Zenodo:** one file, `mgbilby/oga_src-v0.2.0.zip`,
  761,489,280 bytes, md5 `714b9b52abc47c65f26f6f2fefc92520`. Individual works
  are not addressable there; a consumer must download the whole zip.
- **`oga_src` 0.2.0 on GitHub (individually addressable):**
  `https://raw.githubusercontent.com/mgbilby/oga_src/<ref>/data/<file>`. I
  verified that the commit-SHA form resolves for the Anabasis file
  (`…/223d2258745ecf35e7659191080f93640fe900c1/data/tlg0032.tlg006…`). A tag
  can be moved; a commit SHA cannot. **Recommendation:** pin the commit SHA
  and check size and SHA-256 at import, as the existing spec does.
- **Sizes and digests:** every file's byte count from the download equals the
  size in the GitHub tree (2,000 of 2,000). The per-file SHA-256 is in the
  companion TSV. The Anabasis file is
  `data/tlg0032.tlg006.perseus-grc2_oga020_src.conllu`, 7,438,037 bytes,
  SHA-256 `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`,
  which matches the pin in `AnabasisBook1`.
- **Largest files:** `tlg9010.tlg001` (Suidas) is 71,312,662 bytes; the
  Homeric scholia split halves are 48–58 MB. The total of all 2,000 files is
  4,364,430,052 bytes, about 4.1 GB, versus the 761 MB zip.
- **Upstream OGA 0.2.0 (not per file):** the 8.6 GB zip is the PAULA/LAULA-style
  standoff corpus that `oga_src` converts. It is not needed for the shelf.

## Release cadence and stability

- **OGA corpus:** 0.1.0 on 2023-07-18 (preferred citation: the arXiv paper),
  0.2.0 on 2024-11-24. Roughly 16 months apart.
- **`oga_src`:** one release, v0.2.0, on 2025-04-11. No later tag or Zenodo
  version exists as of 2026-10-25. The repository had commits after the
  release (2025-04-11 README; 2026-10-16 funding file) but no data change: all
  2,000 data blobs on `main` match the tag.
- **Stability:** Zenodo DOIs are versioned and not replaced; the GitHub tag
  is mutable in principle, so pin the commit SHA. The upstream Perseus XML has
  changed since the snapshot (for example the Anabasis file's revision history
  records a "citeStructure refsDecl" change dated 2026-08-26), so the OGA
  snapshot is the only frozen copy.

## Inventory

Counts from the companion TSV. "Token" means a CoNLL-U token line, which
includes elliptical tokens.

| Prefix | Files | Works (CTS URN) | Tokens | Bytes |
| --- | ---: | ---: | ---: | ---: |
| `tlg` (TLG author IDs: 363) | 1,812 | 1,811 | 37,974,840 | 4,131,785,028 |
| `pta` (Patristic Text Archive) | 182 | 182 | 2,087,114 | 227,931,835 |
| `sto` | 4 | 4 | 40,972 | 4,655,402 |
| `ggm` | 1 | 1 | 454 | 47,080 |
| `ogl` | 1 | 1 | 109 | 10,707 |

Edition tags (from the file name) and their share of tokens:

| Tag | Files | Tokens | Meaning |
| --- | ---: | ---: | --- |
| `1st1K-grc1` | 929 (+2 split halves in `tlg5026`) | 24,172,036 | First1KGreek edition |
| `perseus-grc2` | 702 | 9,964,328 | Perseus edition (version 2) |
| `pta-grc1` | 181 | 2,085,133 | Patristic Text Archive |
| `opp-grc1` / `opp-grc2` | 129 / 12 | 1,265,803 / 263,352 | `opp` editions (source not checked) |
| `perseus-grc1` | 18 | 683,300 | Perseus-sourced edition, tag `grc1` |
| `perseus-grc3` / `perseus-grc4` | 20 / 1 | 432,667 / 203,075 | Perseus-sourced edition, tags `grc3` / `grc4` |
| `1st1K-grc2`, `pta-grc2`, `digicorpus-grc2`, `cypria-grc1`, `INTF-grc1` | 2 / 1 / 1 / 1 / 1 | — | single files (sources not checked) |

The tag is the file-name segment after the URN. It follows the canonical-greekLit
naming (`<URN>.perseus-grc2.xml`); I confirmed this only for the Anabasis
Perseus file, not for the other tags.

Largest works by tokens (edition as in the file name):

| URN | Edition | Author | Title (OGA abbreviation) | Tokens | Sentences |
| --- | --- | --- | --- | ---: | ---: |
| tlg9010.tlg001 | 1st1K-grc1 | Suidas | Suid. | 621,704 | 86,985 |
| tlg5026.tlg001 | 1st1K-grc1 (src02) | Anonymi | Sch.Homer. Il. | 551,795 | 59,447 |
| tlg4013.tlg004 | 1st1K-grc1 | Simplicius | Arist.physic. | 489,248 | 19,983 |
| tlg5026.tlg001 | 1st1K-grc1 (src01) | Anonymi | Sch.Homer. Il. | 457,058 | 44,601 |
| tlg3135.tlg001 | 1st1K-grc1 | Joannes Zonaras | Epit.Hist. | 395,851 | 17,691 |
| tlg0732.tlg004 | 1st1K-grc1 | Alexander of Aphrodisias | Arist.metaphys. | 383,336 | 16,789 |
| tlg4090.tlg001 | 1st1K-grc1 | Cyril of Alexandria | XII.Proph. | 361,142 | 15,964 |
| tlg0543.tlg001 | perseus-grc2 | Polybius | Hist. | 351,375 | 12,963 |
| tlg4015.tlg009 | 1st1K-grc1 | John Philoponus | Arist.physic.com. | 349,320 | 15,860 |
| tlg0526.tlg001 | perseus-grc2 | Flavius Josephus | Antiq. | 343,820 | 12,704 |
| tlg0008.tlg001 | perseus-grc3 | Athenaeus of Naucratis | Deipn. | 322,758 | 20,406 |
| tlg0081.tlg001 | perseus-grc2 | Dionysius of Halicarnassus | D.H. | 318,791 | 10,755 |

Classical-prose works the shelf is likely to want are in the set, for example
Plato *Rep.* (`tlg0059.tlg030`, 109,334 tokens) and *Leg.* (`tlg0059.tlg034`,
124,997), and Xenophon *Anabasis* (`tlg0032.tlg006`, 65,022 tokens, 3,813
sentences, 622 elliptical tokens).

Author and title come from the first `# source` header of each file. They are
OGA's abbreviations, not full titles, and author strings vary (354 distinct
values across 2,000 files). Mapping them to display names is a curation task.

### Anabasis (the current pinned file)

- URN `urn:cts:greekLit:tlg0032.tlg006.perseus-grc2`; Edition: Marchant, 1904
  (Perseus XML: Clarendon Press, reprint 1961).
- Token lines: 65,022 across seven books; 10,572 with refs in Book 1; the first
  ref is `Xen. Anab._1.1.1` and the last `Xen. Anab._7.8.24`.
- Book 1 citation range `1.1.1`–`1.10.19` is recoverable from MISC refs.
- Citation check against the Perseus XML: section `1.1.1` in Perseus contains
  both oga sentence 1 and sentence 2, and the MISC refs of those tokens say
  `1.1.1`. The per-token refs are therefore at section level and match the
  printed structure.

## Dialect uniformity

Reference: `tlg0032.tlg006.perseus-grc2_oga020_src.conllu`. Every line of all
2,000 files was streamed and checked for: ten tab-separated columns on token
lines; a nine-character postag; `|ref=` in MISC; the `# source` / `# text` /
`# sent_id` triple per sentence; and no stray comments.

**Uniform across the release:**

- Ten columns on every token line, except one line in `tlg2934.tlg066` (the
  stray-`J` line below).
- A nine-character postag on every token line in all 2,000 files.
- Sentence headers in the same three-line order in every file except the
  classes below.
- POS letters are the AGDT set (`n v a d l g c r p m i u x -`, plus `e` in one
  file); the header and token format is the same across all authors.
- Elliptical tokens are a surface `[n]` with POS `-` and postag
  `---------` (Anabasis: 622). Twenty-seven files have none.
- Lemmas carry Morpheus homograph digits (for example `ἄπειμι2`), which the
  adapter's `decodeLemma` already splits.

**Deviations** (all from the full streamed check):

1. **Duplicated `# source` line** in every sentence of six files: `tlg0007.tlg106`,
   `tlg0061.tlg001` (Pseudo-Lucian, *Asinus*), `tlg1805.tlg001`,
   `tlg2200.tlg00447`, `tlg2949.tlg001` (all `1st1K-grc1`/`opp-grc1`/`perseus-grc2`
   editions), and `pta9999.pta083`. Example: `tlg0061.tlg001` has
   `# source = Pseudo-Lucian, Asin._1` twice before `# text`. The current
   adapter's header state machine (`ogasrc.go`, `acceptHeader`) accepts only
   `source`, then `text`, then `sent_id`, so these files would stop the import
   with `source_malformed`.
2. **Stray `J` before the first header** in `tlg2934.tlg066` (John of Damascus,
   *Barlaam and Ioasaph*, `perseus-grc2`, 8,662,374 bytes). Line 1 is
   `J# source = John of Damascus, Barl.Joas. praef.1`. The first sentence
   therefore has no source header and its line is a token-like line. The
   adapter refuses it.
3. **No token refs** in `tlg0007.tlg082` (Plutarch, *Apophthegmata Laconica*,
   `perseus-grc3`, 1,509,213 bytes): all 14,283 token lines have MISC
   `t_N|t_M` with no `|ref=`. Sentence source headers still give locations
   (`Apophth.Lac._1.1`). The adapter refuses it as a missing citation. This
   file is a different edition from the sibling `tlg0007.tlg082a` and `…082b`
   files, which do carry refs.
4. **Greek-letter relation labels** in 26 files (`AuxΚ` in 23 files, `AuxΖ` in
   3; `AuxΖ` has 4 tokens in the Anabasis), homoglyphs of `AuxK` and `AuxZ`. The adapter reads relations only for the `AuxC` check
   in `mapUPOS`, so these do not break the parse, but they are not the same
   string as the Latin labels.
5. **POS `x` on real tokens** in 1,186 files. Example from `ggm0001`:
   `3	3	x--------	AuxG`, a numeral. The adapter's `isEllipsis` treats POS
   `x` as an ellipsis (`ogasrc.go:551–553`), so these tokens would be counted
   as elliptical and skipped in the reconstructed text. The Anabasis has no
   `x` tokens, so the pinned file is unaffected. This is a bug to fix before
   the adapter is used on other works.
Deviations 1–3 are one-line-per-file parse failures. Deviations 4–5 do not
block a parse but change interpretation.

## Citation recoverability

- **Per token:** MISC `ref=<abbrev>_<citation>` on 1,999 files. Citation depth
  varies: the Anabasis uses three levels (`1.1.1`); the Pseudo-Lucian file uses
  one (`Asin._1`–`Asin._56`); the Anonymous Homeric scholia use three with a
  letter (`Il._6a.24.61`). Splitting on the last underscore works for all three
  examples; the other 1,996 files were not parsed individually.
- **Per sentence:** the `# source` header carries the author, an abbreviated
  title and a location. The location is the first (or representative) passage,
  not an exact range, so it is not a reliable unit boundary. Use the token refs.
- **Mechanical derivation of a shelf:** for each work, a unit is a citation
  range over token refs in document order. For the Anabasis Book 1 this gives
  `1.1.1`–`1.10.19`. Ranges spanning a sentence that crosses a boundary (for
  example `1.2.17`–`18`) need the same sentence-splitting logic the current
  adapter uses.

## Licence and provenance

- **`oga_src`:** `license.md` in the repository says the OGA corpus is
  "archived under a CC BY-SA 4.0 international license" and that the derived
  CoNLL-U files are "also archived under a CC BY-SA 4.0 license." The Zenodo
  record for 0.2.0 has licence `cc-by-sa-4.0`. The GitHub API reports the
  repository licence as `NOASSERTION`.
- **OGA corpus record (Zenodo 14206061):** licence field `other-at`, not a CC
  identifier. The `oga_src` file says the corpus is CC BY-SA 4.0, so the two
  should be reconciled before Mouseion relies on a licence claim from the
  corpus record.
- **Upstream:** Perseus canonical-greekLit and First1KGreek are CC BY-SA 4.0
  (repository licence; the Anabasis XML `<availability>`).
- **Arxiv paper:** the Celano paper is marked CC BY-NC-SA 4.0. That licence
  applies to the paper, not the corpus.
- **Annotation provenance (for a learner):** morphosyntax and lemma are
  automatic (COMBO parser trained on AGDT 2.1; Celano 2024, §4.4). Tokenization,
  sentence boundaries and CTS citation are rule-based (§4.2, §4.3, §4.5). The
  paper's Table 1 reports F1 for COMBO trained on AGDT 2.1: UPOS 94.04, XPOS
  85.41, lemma 88.95, LAS 63.55. Lemma and dependency-label errors should be
  expected at learner-relevant rates. The OGA 0.2.0 annotation is not described
  in any source read here; the `oga_src` README says some authorship and date
  data are "sometimes provisional".
- **Lemma numbering:** the homograph digits are Morpheus-style. The related
  research branch notes that Morpheus-derived decodings have their own rights
  position, so this inventory does not resolve it.

## Method

1. `api/records/15199546` and `api/records/14206061` on Zenodo; the versions
   endpoint for each.
2. `api.github.com/repos/mgbilby/oga_src` tags, releases, contents, license,
   and recursive git trees for `v0.2.0` and `main`.
3. For each of the 2,000 `.conllu` files on the tag: streamed from
   `raw.githubusercontent.com` once, SHA-256 computed on the bytes
   received, and checked with an awk script for column counts, postag length,
   MISC `ref=`, header structure, POS and relation sets, first and last ref.
   The byte count matched the GitHub tree size for every file.
4. Sampled files (Anabasis, the Pseudo-Lucian file, the Plutarch file with no
   refs, the John of Damascus file with the stray `J`, the `ggm0001` file with
   POS `x`) were downloaded in full and checked by hand.
5. Perseus XML for the Anabasis: section structure checked against the
   sentence-to-ref mapping.

Scratch files were kept under `~/.cache/scratch/wf2293/` and deleted at the end
of the task.

## Open items

- The Patristic Text Archive licence and the OGA 0.2.0 corpus README were not
  checked (see [Sources checked](#sources-checked)).
- No view of the OGA 0.2.0 annotation provenance (manual corrections, if any)
  was available outside the 8.6 GB zip. Treat automatic annotation as the
  working assumption.
- The `e_N` / `[n]` ellipsis difference between editions and the POS `x`
  handling need a decision in the adapter before a second work is imported.
- Display names for authors and titles (354 author strings, OGA abbreviations)
  are a curation task, not a mechanical one.
