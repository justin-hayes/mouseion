# French Kaikki raw Wiktextract: rights basis for the English raw dump

**Research date:** 2026-10-17 (revision 2 of the same-day note; supersedes revision 1).

**Tickets:** [#2155 Obtain a documented French resource rights basis](https://github.com/justin-hayes/mouseion/issues/2155) (this research);
[#2153 Confirm French resource provenance and permitted uses](https://github.com/justin-hayes/mouseion/issues/2153) (prior provenance note);
[#2141 Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141) (adoption decision gate).

**Repository base:** `6e02d180b1015536127839e0a45953684c0e5a83`.

## Status

**Evidence and conditional grant analysis; no adoption conclusion.** This note sets out what
the sources establish for the raw English Wiktionary dump published by Kaikki.org
(`raw-wiktextract-data.jsonl.gz`), and what remains open. It does not decide adoption
and does not claim a complete rights basis for every field. Adoption belongs to a human decision in
[#2141 Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141).

**Scope.** Personal, local derivation and refresh of an index from that file, through
the same provider and export path that Italian uses today. Not public bundles,
redistribution, or commercial hosting. See [Existing grants on the French path](french-equivalent-use-rights.md)
for the completed companion license-text review. Stanza is out of scope here.

**What changed from revision 1.** (a) First-party Wiktextract and Kaikki issues were searched;
two Kaikki-side statements were found (§2). (b) The claim that imported or third-party text
is "not covered" by the contributor grant was wrong as stated; it is corrected (§3.2).
(c) "No checksum is published" is replaced by a scoped statement (§1). (d) The
sample counts were withdrawn and replaced by one deterministic, documented probe (§4).
(e) French is separated from Italian: Italian runs in code today; French does not (§5).

## Evidence labels

| Label | Meaning |
|---|---|
| **[E]** | Established from a primary source read or fetched in this pass, with the source ID given. |
| **[E-prior]** | From revision 1 or the prior provenance note; not re-fetched here. |
| **[M]** | HTTP headers or registry/API metadata. Not checked against file bytes. |
| **[P]** | Observed in the bounded probe in §4. Only the first 3,000,000 bytes were read. |
| **[I]** | Inference the sources do not state. |
| **[R]** | Reading of an express grant or condition as written. Routine; no separate opinion needed. |
| **[J]** | Legal judgment that the license-text review or a human must make. Named where it arises. |
| **[F]** | Material factual gap that a source could settle but this pass could not. |
| **[D]** | Human decision (adoption, scope, accepted risk). |

## Sources

| ID | Source | Access | Used for |
|---|---|---|---|
| S1 | Kaikki raw-data page, <https://kaikki.org/dictionary/rawdata.html> | fetched 2026-10-17 [E] | Release dates, extractor footer, size, download links |
| S2 | Kaikki home, <https://kaikki.org/> | fetched 2026-10-17 [E] | Self-description; no license statement |
| S3 | Kaikki Italian index, <https://kaikki.org/dictionary/Italian/index.html> | fetched 2026-10-17 [E] | "post-processed" vs. "unprocessed" wording |
| S4 | Raw URL `https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz` | HEAD [M]; `Range: bytes=0-2999999` probe [P] (§4) | Byte size, `last-modified`, `etag` (not a hash) |
| S5 | [Torrent for kaikki.org Wiktextract raw data? — collaborator reuse statement](https://github.com/tatuylonen/wiktextract/issues/1721#issuecomment-6208854795); [entry-order explanation](https://github.com/tatuylonen/wiktextract/issues/1721#issuecomment-5992128622) | `gh issue view` and comment API checked 2026-10-17 [E]; author association `COLLABORATOR` | Affirmative reuse/distribution statement in the raw-data thread; file order |
| S6 | [Add some sort of metadata to the extracted dumps — private regeneration code](https://github.com/tatuylonen/wiktextract/issues/1601#issuecomment-5535824753); [integration follow-up](https://github.com/tatuylonen/wiktextract/issues/1601#issuecomment-5580807681) | `gh issue view` and comment API checked 2026-10-17 [E] | Regeneration code, hashes, dump date |
| S7 | Wiktextract `README.md` at `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a` | read at pinned ref [E] | Package license (MIT); pre-extracted-data pointer; citation request |
| S8 | Wiktextract `LICENSE` at `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a` (blob `83aa2ab…`, 1,291 bytes) | read [E] | MIT; `tests/` carve-out to Wiktionary license |
| S9 | Wikitextprocessor `LICENSE` at `e3d6d4edb77618f4d6680edc66e3f774bea59820` | read [E] | Scribunto GPL carve-out; `tests/` carve-out |
| S10 | Wikimedia Terms of Use §7 "Licensing of Content", <https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use> | fetched 2026-10-17 [E] | Contributor grant; attribution; import rule; database waiver |
| S11 | Wikimedia dumps legal page, <https://dumps.wikimedia.org/legal.html> | fetched 2026-10-17 [E] | Dump-level license statement; residual-infringement disclaimer |
| S12 | Wikimedia `enwiktionary/20260901/dumpstatus.json` | fetched 2026-10-17 [E] | Upstream file size, sha1, md5, job completion time |
| S13 | Wikimedia `enwiktionary/` listing and `20260902/` | `20260902/` returns HTTP 404 (directory and `dumpstatus.json`) [E] | Upstream date mapping |
| S14 | Mouseion `nlp/scripts/derive_dictionary_index.py` at `6e02d180` | read locally [E] | Language gate, example copy, metadata, download |
| S15 | Mouseion `internal/enrichment/llm_codec.go` (lines 176–196) and `internal/enrichment/lexical.go` (lines 30, 277, 388) at `6e02d180` | read locally [E] | Where `Examples` go |
| S16 | Mouseion `internal/dictionary/index.go` (lines 262, 337) at `6e02d180` | read locally [E] | Where `Examples` are used for matching and identity |
| S17 | Wiktextract LREC 2022 paper, <http://www.lrec-conf.org/proceedings/lrec2022/pdf/2022.lrec-1.140.pdf> | downloaded; best-effort text extraction (fonts garbled) [E] | Architecture only; no license statement recovered; weak |

## Bottom line

1. **Release identity** is consistent across S1, S4, and the footer: dump labelled
   2026-09-02, extracted 2026-10-03, Wiktextract `1a05e46`, Wikitextprocessor `e3d6d4e`.
   The Wikimedia input file for that label is not identified (§1.2).
2. **Kaikki-side reuse evidence exists.** A repository collaborator
   says the JSON output is "open source and libre (thanks to the licenses used by
   Wiktionary)" and explicitly says torrenting it is "completely ok" (S5). In a thread
   about raw data this is affirmative support for reuse, not merely silence. It does not
   provide a release-specific rights inventory or separately enumerate database rights (§2).
3. **The contributor grant is the evidenced basis for contributor-originated text**
   (S10, S11). That is a grant, not a per-entry guarantee.
4. **Imported and quoted text needs a per-item basis, which the current output does not
   carry.** Some imports are CC BY-SA-compatible and some quoted works are public domain.
   The sampled raw quotations carry bibliographic `ref` text but no resolved per-item
   license/exception status; the derived index drops that citation (§3.2, §4.3).
5. **Current pipeline behavior:** Italian is live (`LANGUAGES` includes `it`); French is not
   (§5). The derivation copies every example text, regardless of type, and drops the citation.

## 1. Release identity and checksums

### 1.1 Kaikki release

| Field | Value | Source |
|---|---|---|
| Release label | "extracted from the enwiktionary dump dated 2026-09-02"; structured data "extracted on 2026-10-03" | S1, S3 [E] |
| Extractor footer | "wiktextract (1a05e46 and e3d6d4e)" | S1, S3 [E] |
| `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a` | Wiktextract merge commit, 2026-09-21T09:03:13Z | `gh api repos/tatuylonen/wiktextract/commits/…` [M] |
| `e3d6d4edb77618f4d6680edc66e3f774bea59820` | Wikitextprocessor merge commit, 2026-09-18T04:36:08Z. Not present in `tatuylonen/wiktextract` (HTTP 422). | `gh api` [M]; [E] for the 422 |
| Raw `.gz` size | 2,981,058,381 bytes | S4 [M]; `content-range` total in §4 [P] |
| Raw `last-modified` / `etag` | Sat, 03 Oct 2026 08:24:38 GMT / `"6ac0bbc6-b1af574d"` | S4 [M]. The etag is not a content hash. |
| Checksum at paths examined | **None found** at `raw-wiktextract-data.jsonl.gz.sha256`, `…jsonl.gz.md5`, `…sha256sum`, `/dictionary/SHA256SUMS` (all HTTP 404, checked 2026-10-17). Not exhaustive; no checksum file is linked from S1–S3 [E]. | S1–S3 [E]; paths [E] |

The metadata-sidecar path actually proposed in S6,
<https://kaikki.org/dictionary/raw_wiktextract-data-metadata.json>, also returned
HTTP 404 on the coordinating review (2026-10-17). An open proposal is not itself
evidence that a current release has a sidecar or checksum.

A checksum at the examined paths being absent is a reproducibility gap. It is not
evidence about any license grant, and it does not show the file is unverifiable by
other means.

### 1.2 Upstream Wikimedia input: identity open, checksums available for a candidate

- `enwiktionary/20260902/` returns HTTP 404 for both the directory and `dumpstatus.json` [E] (S13).
- The nearest listed run is `enwiktionary/20260901/`. Its `dumpstatus.json` [E] (S12)
  records, for `enwiktionary-20260901-pages-articles.xml.bz2`:
  - size 1,632,298,458 bytes;
  - sha1 `1ed7d1f8ff20ac07349dabe2a921d411385a1e04`;
  - md5 `fd033eda351cff35429f7dd3598d9ecb`;
  - the `articlesdump` job `status: done`, `updated: 2026-09-02 16:51:59`.
- **[I]** The run finished on 2026-09-02, which may be the source of Kaikki's "dated
  2026-09-02" label. This is a guess from one timestamp. It is not established.
- **[F]** Kaikki has not said which upstream file it read. Until it does, the sha1 above is a
  candidate, not a verified checksum of Kaikki's input. This matters for reproduction, not
  for the grant analysis.

## 2. First-party Wiktextract and Kaikki statements

### 2.1 What was searched

Run 2026-10-17 with `gh search issues` and `gh search code` against `tatuylonen/wiktextract`,
and `gh search issues` against `tatuylonen/wikitextprocessor`. Issue and PR search terms:
license, licensing, licence, copyright, CC BY-SA, CC0, Creative Commons, public domain,
share-alike, ShareAlike, attribution, terms of use, GFDL, redistribut, commercial,
database, data dump, data is licensed, Wiktionary license, open source and libre, libre,
kaikki license, raw data license. Code search terms: license, Wiktionary license,
CC BY-SA, CC-BY-SA, copyright, Creative Commons, database rights, kaikki license.

Results: the GitHub search API returns at most the first matches per query, so this is
not exhaustive. Relevant hits were only [tatuylonen/wiktextract#1721 Torrent for kaikki.org Wiktextract raw data?](https://github.com/tatuylonen/wiktextract/issues/1721) and [tatuylonen/wiktextract#1601 Add some sort of metadata to the extracted dumps](https://github.com/tatuylonen/wiktextract/issues/1601) (below). The repo-owner list
(`gh repo list tatuylonen`) contains no `kaikki` repository. The site-regeneration code
is described as private (S6) [E].

### 2.2 Statements found

**S5 — [tatuylonen/wiktextract#1721 Torrent for kaikki.org Wiktextract raw data?](https://github.com/tatuylonen/wiktextract/issues/1721), comment by
`kristian-clausal`, 2026-10-16** (the commenter says "I talked with Tatu" and "we update
the raw data constantly", so they describe themselves as working on the Kaikki side):

> "The outputted json data is open source and libre (thanks to the licenses used by
> Wiktionary), so it is completely ok for someone to torrent it, but that would mostly
> be an archival thing…"

- **What it establishes [E]:** a repository collaborator, discussing raw Kaikki data and
  reporting a conversation with Tatu, expressly endorses distribution and points to the
  Wiktionary licenses as its basis. This materially improves the earlier investigation's
  finding of silence on the examined web pages.
- **Limits:** it names no license version and supplies no complete release-specific list
  of third-party notices, independent additions or database rights. The thread's raw-data
  context supports relevance to the selected input; it is not a blanket assurance that
  every quotation in it has a reusable license. Do not dismiss a useful publisher-side
  statement merely because it is a comment, or treat it as a rights warranty [R/J].

**Same thread, 2026-10-05, same commenter:** "The order of entries in the data file is
random, because we use multiprocessing." This bears on sampling (§4): the head of the
file is an arbitrary subset.

**S6 — [tatuylonen/wiktextract#1601 Add some sort of metadata to the extracted dumps](https://github.com/tatuylonen/wiktextract/issues/1601), comments by
`kristian-clausal`:**

- 2026-03-02: a sidecar metadata file is planned with "date of generation", "wiktionary
  dump file date", "maybe wiktionary dump file hash", "wiktextract commit hash", and
  "wikitextprocessor commit hash". Status: open; not implemented at the time of checking.
- 2026-09-04: "the metadata stuff would probably not be part of wikitextprocessor or
  wiktextract at all, but the private kaikki.org site regeneration code, which has all
  the shell-scripts that create the kaikki.org dictionaries."
- 2026-09-04, same comment: "The dump hash would not be calculated, I just thought it was
  probably scrapeable from the download links on the Wikimedia side, otherwise it'll
  just be the dump-date and other meta-data taken from the dump-file's contents (there
  has to be some, right?)." Read in context this is ambiguous about whether the output
  hash is planned; this note does not rely on it.

**What S6 establishes [E]:** the regeneration code is described as private. The exact
release configuration was not recovered in this pass; this does not prove no other
published evidence could establish it. This is a reproducibility question.

### 2.3 What the first-party evidence does and does not give

- **Given:** the Kaikki side characterizes its JSON output as open because of the
  Wiktionary licenses (S5). The Wiktextract package is MIT (S7, S8), and the README says
  so. The README describes the `tests/` carve-out (S8, S9).
- **Not found:** a release-specific notice enumerating independent database or
  compilation rights; a statement on whether raw records are contributor text, quotations,
  or other content; any third-party clearance.
- **[J]** Whether the Kaikki-side characterization is enough to rely on for the output
  layer depends on whether any part of the output is a separate protectable contribution
  (§3.3). That question is for the license-text review and then a human.

## 3. Grants by layer

### 3.1 Contributor-originated Wiktionary text

- **[E]** Terms of Use §7 (S10): "When you submit text to which you hold the copyright,
  you agree to license it under: Creative Commons Attribution-ShareAlike 4.0 International
  License ('CC BY-SA 4.0'), and GNU Free Documentation License ('GFDL') (unversioned…).
  Reusers may comply with either license or both."
- **[E]** Dumps license page (S11): "all original textual content is licensed under the
  GNU Free Documentation License (GFDL) and the Creative Commons Attribution-Share-Alike
  4.0 License."
- **[R]** For text a contributor originated, the grant to reusers is express and the
  reuser's option is stated. Attribution, notice, and modification duties are set out in
  S10 and are conditions of the grant. Applying those conditions to a particular index or
  export is the conditional question in §6.
- **[F]** The per-entry status is not checked. This pass did not sample any entry's page
  history to confirm which text is contributor-originated.

### 3.2 Imported and quoted text (corrected)

Revision 1 said imported or third-party text is "not covered by the contributor grant."
That was too broad. The correct statement:

- **[E]** ToU §7 has an import rule (S10): an importer may import text found elsewhere if
  "the text is available under terms that are compatible with CC BY-SA", and "You shall
  not import content that is available solely under GFDL."
- **[R]** So a quoted or imported passage is covered by the contributor grant only to the
  extent it is itself available under a compatible license. A passage that is
  CC BY-SA-compatible (licensed by its own source) is not excluded merely because it came
  through an import. A passage with no compatible license is not covered by Wikimedia's
  grant. A passage from a public-domain work is not covered by any license, because none
  is needed.
- **[R]** "Quotation" is a label on the example type in the output. It is not evidence of
  a license or of its absence. The same type covers a 1310s text (public domain as a work)
  and a 2008 newspaper article (copyrighted unless licensed otherwise). §4.3 gives examples.
- **[J]** Where a public-domain work is quoted from a modern edition, the edition or
  translation may carry its own rights. Whether an excerpt's edition has its own rights is a
  question about each excerpt, not something the type field can answer.
- **[F] The per-item basis is not resolved.** Sampled raw `quotation` examples include
  bibliographic `ref` text identifying a work, but no resolved license/public-domain/
  exception status. The current derived index drops `ref` and retains only the text.
  Revision 1's categorical exclusion from all grants was unwarranted: a quotation label
  is not a legal-status classifier, and a citation is not itself a license.
- **Excluding by type is not a license judgment.** Dropping `quotation` examples removes
  the ones whose status is unrecorded. It also removes some that might be licensed. It
  is a conservative exclusion choice, [D].

### 3.3 Kaikki/Wiktextract structured output

- **[E]** Output fields are structured JSON derived from Wiktionary pages: senses, glosses,
  examples, forms, sounds, and so on (§4 sample [P]).
- **[E]** Kaikki's index page describes the raw file as "unprocessed wiktextract data" (S3).
  Its post-processed files are "merged from other sources" (S3). The raw-vs-merged
  distinction is therefore stated by Kaikki [E].
- **[J]** Whether this structured output is a separate protectable contribution (for
  example a database right in a substantial selection and arrangement, or original text)
  is not stated by any source. A format transformation of licensed text does not, by
  itself, create a new right to block reuse of the licensed content. Whether any separate
  right exists in the output is for the license-text review and then a human [F].
- **[F/J]** No examined source separately enumerates a Kaikki database-right claim or
  waiver. The collaborator's reuse statement is affirmative evidence, not a prohibition.
  A further authoritative notice, maintainer clarification **or supported human/legal
  interpretation** of existing grants and any protectable layer may resolve this;
  contacting Kaikki is not the only possible route.

## 4. Bounded probe of the raw file

### 4.1 Command

Hard cap at the transport layer and a second cap in the parser. The data file is outside
the repository; the parser reads only that local range.

```sh
curl -sS --max-time 120 --max-filesize 3000000 -r 0-2999999 \
  -D headers.txt -o range.gz \
  -w 'http_code=%{http_code} size_download=%{size_download}\n' \
  https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz
python3 -I probe_kaikki.py range.gz probe.json
```

Save the parser in Appendix A as `probe_kaikki.py` in a task-specific scratch
directory before running these commands. The recorded range hash, not the moving
URL alone, identifies the exact inspected prefix.

### 4.2 Transport result [P]

- `http_code=206 size_download=3000000`.
- `content-range: bytes 0-2999999/2981058381`.
- `last-modified: Sat, 03 Oct 2026 08:24:38 GMT`; `etag: "6ac0bbc6-b1af574d"`.
- SHA-256 of the 3,000,000-byte range: `d1a32e111c037f3bbf7d733a8e2f2618bef98c3241e61f470c56ab90c578e153`.

### 4.3 Parser result [P]

- Decompression stopped at the cap (`truncated_stream: true`). The unfinished final line
  was not parsed. `complete_records: 2982`; `skipped_lines: 0`.
- **Top-level `lang_code`** (the record's own key, not nested objects): `fr` 109, `it` 50.
  Top five: `en` 824, `fr` 109, `la` 106, `enm` 78, `es` 58.
- **Example types** across all senses' `examples` arrays: `quotation` 2219, `example`
  1194, untyped 253. Example objects with a `ref` key: 2263.
- **Within top-level `fr` and `it` records:** `fr` has 33 `example` and 5 `quotation`
  examples; `it` has 2 `example` and 1 `quotation`. These are small numbers and do not
  support a French or Italian rate.
- **Withdrawn:** revision 1 and the earlier draft of this section reported counts of
  1,475 `quotation`, 605 `fr`, and 534 `it` from a 2,000,000-byte read. They were
  substring counts and included nested `lang_code` fields (for example in related or
  translation objects). The same 3,000,000-byte range gives 872 `fr` and 747 `it`
  substring matches against 109 and 50 top-level records. Those figures are not used.
- **Sampling limit [E, S5]:** the file's entry order is random, so the head is an
  arbitrary subset. The probe says nothing about the proportion of quotations in the full file.

### 4.4 Redacted record examples (source pages checked)

| Language | Headword and POS | Example type, length | Citation (redacted) | Source page |
|---|---|---|---|---|
| fr | *ablation*, noun | `quotation`, 155 chars | 2008 newspaper article (Le Devoir); author and title redacted | <https://en.wiktionary.org/wiki/ablation> (HTTP 200 [E]) |
| it | *abate*, noun | `quotation`, 107 chars | 1310s medieval poem (public-domain work) in a 1994 modern edition; edition details redacted | <https://en.wiktionary.org/wiki/abate> (HTTP 200 [E]) |

Both examples show the `quotation` type covers unlike cases (§3.2). Neither page was
checked for history or per-quotation licensing.

## 5. Pipeline behavior: Italian (live) and French (planned)

- **Italian is live.** `nlp/scripts/derive_dictionary_index.py:21` sets
  `LANGUAGES = {"de", "it", "el"}`. Line 430 skips records whose language is not in that
  set [E]. The same raw file feeds Italian and German and Greek.
- **French is not live.** `fr` is not in `LANGUAGES`. No French branch was found in
  `nlp/scripts/`, `internal/dictionary/`, `internal/cardexport/`, or `internal/enrichment/`
  [E]. The French dictionary path is therefore **planned**: it would add `fr` to the set.
  No French index exists in the repository. The French-edition extract (`fr-extract.jsonl.gz`
  from frwiktionary, listed on S1) is a different input and is not evaluated here.
- Because French would use the same record shape and the same file, rights conclusions
  for the English dump apply to its `fr` records; they do not add a French-only category.

### 5.1 Pipeline behavior that applies to Italian today

- **Copy of example text, all types, no citation** (`derive_dictionary_index.py:313–318`).
  `sense_from` appends `example["text"]` for every example object with a string `text`,
  regardless of `type`. The `ref` field is never read [E].
- **Download without checksum** (lines 355–366): fetches the fixed URL with `urlopen`,
  writes to a cache file, and does not compute a digest [E].
- **Metadata written** (lines 517–527): `license` is `"Wiktionary-derived data: CC BY-SA 4.0 / GFDL"`;
  `attribution` is `"Wiktionary contributors; CC BY-SA 4.0 / GFDL"`; `source` is
  `"Kaikki.org Wiktextract enwiktionary JSONL"`; plus `dump_date`, `extraction_date`,
  `wiktextract_commit`. No Wikitextprocessor commit, no input filename, no digest [E].
- **Date defaults** (lines 601–603): `--dump-date` defaults to `""`, `--extraction-date`
  to `date.today()`, and `--wiktextract-commit` to `""` [E]. A run today records 2026-10-17
  as the extraction date, not Kaikki's 2026-10-03. That is a labeling error and is
  independent of rights.
- **Where `Examples` go** [E]:
  - Evidence identity hash and sentence-overlap matching: `internal/dictionary/index.go:262, 337`.
  - Word matching for lexical senses: `internal/enrichment/lexical.go:277`.
  - Copied into lexical senses and serialized as `candidate_senses` in the LLM request
    payload: `internal/enrichment/llm_codec.go:176–196`, `lexical.go:388`.
  - No card-export code references `Examples` (search of `internal/cardexport/`). The
    card export path carries the gloss, not the examples [E].
  So the example quotation text is sent to a model provider on the contextual-gloss
  path. Whether provider terms matter is outside this note.

## 6. Use status: personal derivation only

Status only; no clearance and no adoption decision.

| Question | Evidence | Status |
|---|---|---|
| Copy and adapt contributor text locally to build an index | §3.1 (S10, S11); companion [license-text review](french-equivalent-use-rights.md) | CC BY-SA §2 expressly grants reproduction/adaptation. Wholly private derivation is not public Share under §1(k), so CC §3 attribution/ShareAlike duties do not arise merely from creating the local index. Keep this distinct from broader ToU reuse wording and source exceptions. |
| Include imported or quoted text (`quotation` examples) | §3.2 [F]; §4.4 | **Per-item basis not recorded.** Some items may be CC BY-SA-compatible; some public-domain; some unknown. Option: exclude by type [D], or record per-item basis [F]. |
| Keep the `ref` citation | §5.1 | **Currently dropped.** Keeping it would help any future per-item check. |
| Use the Kaikki-side layer | §2, §3.3 | Kaikki characterizes output as open via Wiktionary licenses (S5). No data license terms. Whether separate rights exist is **[F] and [J]**. |
| Identify the upstream input | §1.2 | **[F]** Kaikki has not identified the Wikimedia file. Reproducibility, not a grant question. |
| Reproducible provenance | §1, §2.2 | **Gap:** no checksum at examined paths; no Wikitextprocessor commit or input filename recorded; extraction date defaults to run date; regeneration config is private (S6). |
| Attribution per S10 | §3.1 | Current `AttributionNotice` (`internal/dictionary/attribution.go:4`) names the sources and has no per-entry link. Whether a deck-level notice meets the medium-appropriate standard is **[J]**. |

## 7. Resolved facts and narrow remaining ambiguity

**Resolved (with source):**
- Release labels, extractor pair, and commit dates (§1.1; S1, S4, S6, S7, GitHub API).
- The raw file's byte size, last-modified time, and etag; no checksum at examined paths (§1.1).
- Kaikki characterizes the output as open via Wiktionary licenses (S5); its regeneration
  code is private (S6).
- The contributor grant for contributor-originated text (S10, S11).
- Import rule and how it limits quotations (S10); the corrected statement in §3.2.
- Probe transport and parser facts (§4.2, §4.3); the top-level counts; the withdrawn
  counts (§4.3).
- Italian is live in code; French is not (§5).
- `Examples` are copied for all types and the citation is dropped (§5.1).

**Narrow remaining ambiguity:**
1. **Independent Kaikki-side rights, if any.** S5 supports reuse of the raw output under
   Wiktionary's licenses, but is not a release-specific inventory. Do not assume a separate
   right exists merely because there is structured output. A supported interpretation or
   further authoritative evidence may settle any material residual coverage question.
2. **Which Wikimedia file was the input** (checksum candidate in §1.2). [F] Reproducibility.
3. **Per-item basis for quotation-type examples.** Each needs its cited work, license or
   public-domain status, and source. [F] Not recorded in the output; per-item checks would
   require sampling pages or edition metadata.
4. **Private derivation under CC BY-SA is answered by the express grant and public-Share
   definition**, not a new question for counsel. See the companion license-text review
   for the separate ToU wording and notice-path caveat.
5. **Whether a deck-level or index-level notice meets the attribution requirement.** [J]
   and then [D].
6. **Whether to exclude quotation-type examples, or keep them with a recorded basis.** [D].

No adoption or universal clearance follows from this note.

## Appendix A: probe script (`probe_kaikki.py`)

```python
"""Bounded probe of the head of the Kaikki raw Wiktextract gzip.

Reads only a local, already-capped byte range (written by curl with -r and
--max-filesize), decompresses it as a stream, and counts only top-level keys.
Lines that do not parse (the final line is truncated by the cap) are skipped.
"""

import gzip
import io
import hashlib
import json
import sys
from collections import Counter

range_path, out_path = sys.argv[1], sys.argv[2]

with open(range_path, "rb") as handle:
    raw_bytes = handle.read()
sample_sha256 = hashlib.sha256(raw_bytes).hexdigest()

lang_counts = Counter()
pos_counts = Counter()
example_type_counts = Counter()
example_type_by_lang = {"fr": Counter(), "it": Counter()}
ref_count = 0
complete_records = 0
skipped_lines = 0
fr_it_examples = {"fr": [], "it": []}

stream = gzip.GzipFile(fileobj=io.BytesIO(raw_bytes))
buffered = b""
truncated = False
while True:
    try:
        chunk = stream.read(65536)
    except (EOFError, OSError):
        truncated = True
        break
    if not chunk:
        break
    buffered += chunk
    *lines, buffered = buffered.split(b"\n")
    for line in lines:
        if not line:
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            skipped_lines += 1
            continue
        if not isinstance(record, dict):
            skipped_lines += 1
            continue
        complete_records += 1
        code = record.get("lang_code")
        lang_counts[code] += 1
        for sense in record.get("senses", []) or []:
            if not isinstance(sense, dict):
                continue
            for example in sense.get("examples", []) or []:
                if isinstance(example, dict):
                    example_type_counts[example.get("type")] += 1
                    if code in example_type_by_lang:
                        example_type_by_lang[code][example.get("type")] += 1
                    if "ref" in example:
                        ref_count += 1
                    if code in fr_it_examples and len(fr_it_examples[code]) < 3:
                        if example.get("type") == "quotation":
                            fr_it_examples[code].append(
                                {
                                    "word": record.get("word"),
                                    "pos": record.get("pos"),
                                    "gloss": (sense.get("glosses") or [""])[0][:80],
                                    "example_type": example.get("type"),
                                    "text_chars": len(example.get("text", "")),
                                    "ref": example.get("ref", ""),
                                }
                            )
        if code in ("fr", "it"):
            pos_counts[(code, record.get("pos"))] += 1

report = {
    "range_bytes": len(raw_bytes),
    "range_sha256": sample_sha256,
    "truncated_stream": truncated,
    "complete_records": complete_records,
    "skipped_lines": skipped_lines,
    "top_level_lang_code_fr": lang_counts.get("fr", 0),
    "top_level_lang_code_it": lang_counts.get("it", 0),
    "top_level_lang_code_top5": lang_counts.most_common(5),
    "example_type_counts_all_senses": example_type_counts.most_common(6),
    "example_refs_all_senses": ref_count,
    "example_type_by_top_level_lang": {k: v.most_common(4) for k, v in example_type_by_lang.items()},
    "fr_it_examples_quotation_sample": fr_it_examples,
}
with open(out_path, "w", encoding="utf-8") as out:
    json.dump(report, out, ensure_ascii=False, indent=1)
print(json.dumps({k: v for k, v in report.items() if k != "fr_it_examples_quotation_sample"}, ensure_ascii=False, indent=1))
```

Run output (`range_sha256` matches §4.2): `complete_records` 2982, `skipped_lines` 0,
`truncated_stream` true, `top_level_lang_code_fr` 109, `top_level_lang_code_it` 50.
