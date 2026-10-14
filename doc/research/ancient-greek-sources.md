# Ancient Greek text repositories and machine-analyzed corpora

**Checked:** 2026-10-14 (upstream state as read on this date; repository `pushed_at`, commit dates, and Zenodo record fields cited below)

**Ticket:** [#2022](https://github.com/justin-hayes/mouseion/issues/2022), child of [#2021](https://github.com/justin-hayes/mouseion/issues/2021).

**Revision:** Revision 2 (this file) supersedes the first resolution of 2026-10-14. It corrects overclaims in revision 1, adds the large automatic corpora GLAUx and Opera Graeca Adnotata (OGA), and records the Diorisis update channel and derivatives. Original findings are kept unless a correction is stated.

**Question:** Which maintained Ancient Greek text repositories and large machine-analyzed corpora can Mouseion responsibly use today, and under what acquisition and redistribution terms?

**Scope limits:** This file records evidence only. It does not choose Mouseion architecture, the MVP, or any acquisition policy. It does not give a legal opinion. Items marked *unknown* were not verified. A stated license is what the upstream repository or record says, not a legal conclusion about what applies.

## Summary

- **Open-licensed TEI text, no linguistic annotation in the sampled files:** Perseus `canonical-greekLit` and Open Greek and Latin `First1KGreek` are both stated as CC BY-SA 4.0 in their repositories and are actively maintained (pushed 2026-10-06 and 2026-09-30). The sampled Greek files contain text and CTS metadata only (no `<lemma>`, `<w>`, or morphological markup). A repository-wide format inventory was not done, so "no annotation" is qualified to the sampled files.
- **Human-reviewed syntax and morphology (small):** Perseus AGDT `treebank_data` v2.1 has 33 Greek text files (32 works) with human-reviewed syntax and Morpheus-assisted, human-corrected morphology. Its root README states CC BY-SA 3.0 US; its v1 documentation states CC BY-NC-SA 2.5. These are two documents for different releases; the scope of each is unresolved (see Licensing).
- **Large automatic corpora (three found, not one):**
  - **Diorisis** (Vatri & McGillivray): 820 TEI XML texts, 10,206,421 words per its figshare v1 record. Per-token analyses are automatic dictionary lookup with optional TreeTagger tie-breaking. Its figshare record says CC BY 4.0 while each text's header says CC BY-SA 3.0 US. Unresolved.
  - **GLAUx** (Keersmaekers et al., KU Leuven / Pedalion): 1,421 texts, 19,498,849 tokens (sum of its `metadata.txt`), automatically annotated for morphology and syntax (a sampled 10-file subset is all `analysis="auto"`). Its repository README states CC BY-SA 4.0 generally with per-text exceptions; the Zenodo v2.1 record says CC BY 4.0. Unresolved, and the per-text licenses include non-commercial and unlicensed rows.
  - **OGA** (Celano, Opera Graeca Adnotata v0.2.0): 1,999 base texts, 40,105,221 tokens, morphosyntax and lemmas automatically produced by a parser and lemmatizer trained on about 1.2 M tokens of AGDT. Its README states CC BY-SA 4.0; its Zenodo record field says `other-at`. Unresolved. The Zenodo archive is 8.6 GB.
- **Human-annotated alternatives with restrictive licenses:** UD PROIEL (CC BY-NC-SA 3.0) and UD Perseus (CC BY-NC-SA 2.5) are maintained but non-commercial by their stated licenses. Gorman's `Greek-Dependency-Trees` has an explicit `TREEBANK LICENSE` file stating CC BY-SA 4.0 (GitHub's null license field is detection, not evidence of no license). Its exclusion below is for other reasons (see Recommendation inputs).
- **Mirror and tooling:** `cltk/grc_text_perseus` mirrors Perseus Greek under a stated CC BY-SA 3.0 US (pushed 2023-02-01). `PerseusDL/morpheus` has an explicit README statement that all contents are CC BY-SA 3.0 US, while GitHub detects no license. Whether that covers code, lexicon data, or both is unresolved.
- **Diorisis update channel:** the updater script points to `crs.rm.it/diorisisCorpusupdates/latest.json`, which returns HTTP 200 and lists 821 entries (versions 1.6 and 1.51). The actual update download is token-gated by the updater script and was **not** used. No public release of the newer Diorisis JSON data was located. The newer version's license is *unknown*.
- **Not verified:** TLG (Thesaurus Linguae Graecae) terms could not be retrieved (HTTP 403 on `stephanus.tlg.uci.edu`).

## Source inventory

Sizes are from GitHub's `size` field (KB) or the Zenodo/figshare `size` field (bytes). "Files" counts XML members excluding `__cts__.xml` CTS metadata. License rows give the **stated** license, and conflicts are listed in the licensing section.

| Source | Layer | Stated license (where) | Coverage (verified) | Format / IDs | Last activity | Archival vs. active |
|---|---|---|---|---|---|---|
| [PerseusDL/canonical-greekLit](https://github.com/PerseusDL/canonical-greekLit) @ `01b725d8` | Text + CTS metadata | CC BY-SA 4.0 (`license.md`; README; GitHub `CC-BY-SA-4.0`) | 1,612 XML files: 810 `perseus-grc*` (Greek), 786 `perseus-eng*`, 16 `1st1K-eng*` copies | TEI EpiDoc; CTS URN `urn:cts:greekLit:tlgNNNN.tlgNNN.perseus-grcN`; `__cts__.xml` per work; 769 distinct Greek work keys | pushed 2026-10-06 | Active |
| [OpenGreekAndLatin/First1KGreek](https://github.com/OpenGreekAndLatin/First1KGreek) @ `03776b39` | Text + CTS metadata | CC BY-SA 4.0 (`license.md`; GitHub `CC-BY-SA-4.0`) | 1,204 XML files: 1,126 with `grc` in the filename (969 `1st1K-grc`, 137 `opp-grc`, 19 `perseus-grc`); 1,091 distinct Greek work keys | TEI EpiDoc; CTS URN `urn:cts:greekLit:ogl0001.ogl001.1st1K-grc1` | pushed 2026-09-30 | Active (Zenodo DOI in README: 10.5281/zenodo.596723) |
| [PerseusDL/treebank_data](https://github.com/PerseusDL/treebank_data) @ `bf4334f0` (AGDT) | Syntax + morphology (human-reviewed, Morpheus-assisted) | Root README: CC BY-SA 3.0 US; `v1/greek/docs/README.txt`: CC BY-NC-SA 2.5; GitHub license `null` | `v2.1/Greek/texts`: 33 files, 32 distinct Greek work keys, 76.5 MB | TB XML: `<treebank version cts>`, `<word id form lemma postag relation head>`; CTS `urn:cts:greekLit:tlgNNNN.tlgNNN.perseus-grcN.tb` | last commit 2024-06-20 | Archival |
| [alevatri/diorisis](https://github.com/alevatri/diorisis) @ `55ecc1d1` (annotation scripts) | Scripts, lexicon tables | none in repo (no LICENSE file; README silent) | `grkLemmata.py`: 112,385 lemma entries; `grkFrm.py`: 911,840 form entries | Python dictionaries | last commit 2018-10-19; `pushed_at` 2024-06-27 | Archival |
| [Diorisis Ancient Greek Corpus](https://figshare.com/articles/dataset/Diorisis_Ancient_Greek_Corpus/6187256) (figshare 6187256, v1) | Text + automatic per-token analysis | figshare record: CC BY 4.0; each text's `teiHeader` licence: CC BY-SA 3.0 US | 820 XML files; 10,206,421 words (figshare description); Homer to 5th c. AD | Single archive `Diorisis.zip`, 194,443,428 bytes; 820 `.xml`, no README or license file inside (central directory) | figshare modified 2023-05-30 | Archival (v1 only found) |
| [figshare 7229162](https://figshare.com/articles/dataset/Diorisis_Corpus_-_Preprocessed_files/7229162) (preprocessed Diorisis input) | Pre-annotation tokenized files | CC BY 4.0 (figshare) | `annoCorpora.zip`, 59,893,269 bytes | Tokenized XML | modified 2023-05-31 | Archival |
| [Zenodo 11261146](https://doi.org/10.5281/zenodo.11261146) `Diorisis.duckdb` (Bilby) | Derived database compilation of the Diorisis XML | **CC BY-NC-ND 4.0** (Zenodo record); description says "The original Diorisis corpus is archived under a CC BY 4.0 international license" | One file, 989,605,888 bytes; version field empty | DuckDB; relation to the JSON channel not verified | published 2024-05-23 | Derived |
| [UniversalDependencies/UD_Ancient_Greek-PROIEL](https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL) | Syntax + morphology (converted from manual) | CC BY-NC-SA 3.0 (UD README metadata; GitHub `NOASSERTION`) | New Testament plus Herodotus selections (UD README) | CoNLL-U | pushed 2026-10-05 | Active |
| [UniversalDependencies/UD_Ancient_Greek-Perseus](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus) | Syntax + morphology (converted from manual) | CC BY-NC-SA 2.5 (UD README metadata; GitHub `NOASSERTION`) | AGDT-derived texts | CoNLL-U | pushed 2026-10-05 | Active |
| [vgorman1/Greek-Dependency-Trees](https://github.com/vgorman1/Greek-Dependency-Trees) | Syntax (one annotator; some pre-parsed then hand-corrected) | `TREEBANK LICENSE` file: CC BY-SA 4.0 (GitHub license `null`) | Prose trees; count not measured | Arethusa/Perseids XML | last push 2022-08-15 | Dormant |
| [PerseusDL/morpheus](https://github.com/PerseusDL/morpheus) | Morphological analyzer code and/or data (scope unresolved) | README: all contents CC BY-SA 3.0 US; GitHub: no license detected | n/a | Source code (lexicon data not located in this repo) | pushed 2019-03-11 | Dormant |
| [cltk/grc_text_perseus](https://github.com/cltk/grc_text_perseus) | Mirror of Perseus Greek text | CC BY-SA 3.0 US (`LICENSE.md`; README) | Mirror of Perseus files (not compared file by file) | Perseus-derived XML | pushed 2023-02-01 | Dormant mirror |
| [alekkeersmaekers/glaux](https://github.com/alekkeersmaekers/glaux) @ `b077d8f6` (2026-02-19) | Text + automatic morphology/syntax (plus manual treebank sources, per-text) | README: "generally … CC BY-SA 4.0", some texts more restrictive, per-text licence in `metadata.txt`; GitHub: no license detected; Zenodo v2.1 record: **CC BY 4.0** | 1,421 texts; 19,498,849 tokens (sum of `TOKENS`); 215,304 `<word>` elements in a 10-file sample | Treebank v2 XML with `<word id form line lemma postag head relation>` per sentence, Unicode NFD; `CITATION.cff` | pushed 2026-02-19 | Active data release (v2.1) |
| [Zenodo 17295426](https://doi.org/10.5281/zenodo.17295426) (GLAUx v2.1; concept `10.5281/zenodo.10948374`) | Archive of the GLAUx repository | **CC BY 4.0** (Zenodo field `cc-by-4.0`) | `glaux-v2.1.zip`, 348,367,270 bytes; v2.1 "identical to 2.0 except escaping" (record description) | ZIP | published 2026-02-19 | Archival snapshot |
| [Zenodo 10948375](https://doi.org/10.5281/zenodo.10948375) (GLAUx v1.0) | Archive | CC BY 4.0 (Zenodo field) | `glaux-v1.0.zip`, 323,213,508 bytes | ZIP | published 2024-04-09 | Archival |
| [alekkeersmaekers/glaux-nlp](https://github.com/alekkeersmaekers/glaux-nlp) | Code for GLAUx annotation | GitHub: no license detected | not inspected | Code | pushed 2026-09-22 | Active |
| [OperaGraecaAdnotata/OGA](https://github.com/OperaGraecaAdnotata/OGA) @ docs (v0.2.0) | Documentation of OGA | README `## Licence`: CC BY-SA 4.0 (`creativecommons.org/licenses/by-sa/4.0/`); GitHub: no license detected | 1,999 base texts; 40,105,221 tokens (README) | Standoff PAULA XML / LAULA XML; ANNIS 4 query interface | pushed 2025-03-29 | Active docs |
| [Zenodo 14206061](https://doi.org/10.5281/zenodo.14206061) (OGA v0.2.0; concept `10.5281/zenodo.8158674`) | Corpus archive | Zenodo field **`other-at`** (not a CC identifier); README and website say CC BY-SA 4.0 | `opera_graeca_adnotata_v0.2.0.zip`, 8,629,032,478 bytes | ZIP | published 2024-11-24 | Archival |
| [Pedalion Trees](https://github.com/perseids-publications/pedalion-trees) | Syntax (human-annotated trees) | `TREEBANK_LICENSE` file: CC BY-SA 4.0 (text of the license); `LICENSE`: MIT, copyright "The Perseids Project" (2018). GitHub reports MIT. | 1,168,591 tokens within GLAUx's `TREEBANK_ANNOTATIONS` column (GLAUx metadata) | Repo-specific | not checked | Not checked |
| [PatristicTextArchive/pta_data](https://github.com/PatristicTextArchive/pta_data) @ `524eb976` (2026-06-18) | Patristic Greek text (`data/`) and automatically tokenized/analyzed files (`analyzed/`) | Per file: licence in each TEI header (README: "individual files are licensed by different Creative Commons Licenses"). Sampled `pta0001.pta025` header: CC BY-SA 4.0 | 766 XML files in `data/` (incl. `__cts__.xml`); 487 XML files in `analyzed/` | TEI (PTA schema); CTS-style paths | pushed 2026-06-18 | Active |
| [Bilby CATnaPS, Zenodo 15508287](https://doi.org/10.5281/zenodo.15508287) | Derived tool data (scope not verified) | CC BY-SA 4.0 (Zenodo field) | One record; contents not inspected | Not inspected | published 2025-05-22 | Not checked |
| [Bilby AGDTmini, Zenodo 15508450](https://doi.org/10.5281/zenodo.15508451) | Derived from AGDT XML and CoNLL-U (record description) | CC BY 4.0 (Zenodo field) | `AGDTmini_demo.tar.gz`, 116,664,702 bytes (demo archive; full data not checked) | CSV, `lemma@XPOS` strings | published 2025-05-22 | Derived |
| [MorphGNT/sblgnt](https://github.com/morphgnt/sblgnt) | New Testament morphology | README: morphological data CC BY-SA 3.0; SBLGNT text subject to the SBLGNT EULA | New Testament only | Text + morphology | not checked | Out of scope (NT) |
| TLG (Thesaurus Linguae Graecae) | Text + annotation (licensed) | **unknown**: `stephanus.tlg.uci.edu` returned HTTP 403 | unknown | unknown | unknown | unknown |

## Licensing by layer

Layers are kept separate: text, linguistic annotation, metadata, and derived data. Each row states what a source says. Where two statements disagree, both are shown, and the scope of each (which release, which layer, which file) is given. A null or undetected GitHub license is recorded as "no license detected", not as "unlicensed".

| Layer | Source | Stated (exact release or record) | Conflict or gap |
|---|---|---|---|
| Text | canonical-greekLit (HEAD `01b725d8`, 2026-10-06) | CC BY-SA 4.0 (`license.md`) | The README also says Tufts holds "overall copyright" and that materials are "provided for the personal use of students, scholars, and the public", with rights "varying" by component. Not reconciled. |
| Text | First1KGreek (HEAD `03776b39`, 2026-09-30) | CC BY-SA 4.0 (`license.md`) | Per-file TEI header licenses were not audited. |
| Text and annotation | AGDT `treebank_data` | Root README (current): CC BY-SA 3.0 US. `v1/greek/docs/README.txt` (v1 documents): CC BY-NC-SA 2.5. | Different documents for different releases. Whether v2.1 Greek inherits either is unresolved. Not treated as one release contradicting itself. |
| Annotation | UD PROIEL, UD Perseus | CC BY-NC-SA 3.0 and 2.5 (UD README metadata) | Non-commercial and ShareAlike; the UD license applies to the UD conversion, and the upstream PROIEL/AGDT licenses were not checked for each UD release. |
| Annotation | Gorman trees (`TREEBANK LICENSE`) | CC BY-SA 4.0 (explicit file) | GitHub reports no license. Detection failing does not mean no license; the file's text governs. |
| Annotation | Pedalion Trees | `TREEBANK_LICENSE`: CC BY-SA 4.0; `LICENSE`: MIT (Perseids, 2018) | Two layers in one repo: code (MIT) and treebank data (CC BY-SA 4.0). GitHub shows MIT. |
| Annotation | GLAUx (automatic; manual treebank sources per row) | README: CC BY-SA 4.0 generally, per-text exceptions. `metadata.txt` per text (see table below). Zenodo v2.1: CC BY 4.0. | Record and README conflict for the same v2.1 release; per-text rows conflict with both (see below). |
| Annotation | OGA (automatic) | README `## Licence`: CC BY-SA 4.0. Zenodo v0.2.0: `other-at`. | Record field does not name a CC license; README governs the documentation, but the record's stated license for the archive is not the same statement. Unresolved. |
| Annotation | Diorisis per-token lemmas/analyses | figshare record: CC BY 4.0; per-text `licence`: CC BY-SA 3.0 US; Zenodo 11261146 (derivative DuckDB): CC BY-NC-ND 4.0 | Three statements about the same content. Scripts repo has no license. |
| Metadata | Diorisis `teiHeader` | CC BY-SA 3.0 US (in each text) | Same as above. |
| Metadata | canonical-greekLit / First1KGreek CTS `__cts__.xml` | Inherit repo license (CC BY-SA 4.0) | Not separately stated. |
| Derived | Diorisis texts drawn from Perseus | Perseus: CC BY-SA 4.0 (current repo), CC BY-SA 3.0 US (cltk mirror; Diorisis text headers) | Versions differ by source and date. Whether any Diorisis-derived statistic inherits ShareAlike is an open legal question (see below). |
| Derived | AGDTmini (Zenodo 15508450) | CC BY 4.0 (Zenodo field) | Derived from AGDT per the record description. Whether a CC BY 4.0 label on a derivative of AGDT's stated terms is consistent is not decided here. |
| Software | PerseusDL/morpheus | README: all contents CC BY-SA 3.0 US (explicit). GitHub: none detected. | The README statement is explicit and is not discarded because GitHub detects no license. Whether it covers code as well as lexicon data is unresolved; lexicon data layers are not in this repo. |

### GLAUx per-text licenses (from `metadata.txt`, HEAD `b077d8f6`)

Computed from the 1,421 rows of `metadata.txt`, summing `TOKENS`:

| `SOURCE_LICENSE` value | Rows | Tokens |
|---|---|---|
| `CC BY-SA 4.0` | 999 | 14,567,637 |
| `NA` | 174 | 2,464,552 |
| `CC-BY-SA 3.0` | 227 | 2,240,012 |
| `CC BY 4.0` | 10 | 2,120 |
| `CC BY-SA 2.0` | 1 | 109,743 |
| `CC BY-NC-ND 3.0` | 1 | 80,361 |
| `CC BY-NC-SA 4.0` | 3 | 9,290 |
| `CC BY-NC-ND 4.0` | 2 | 14,847 |
| `CC BY-NC-ND 2.0` | 1 | 2,702 |
| `CC BY-SA 2.5` | 1 | 1,205 |
| `GNU General Public License` | 1 | 6,369 |
| `OpenEdition Books License` | 1 | 11 |

Within this: all 709 Perseus rows are `CC BY-SA 4.0`; all 286 First1K rows are `CC BY-SA 4.0`; all 200 Wikisource rows are `CC-BY-SA 3.0`. The 174 `NA` rows are mainly web sources (documentacatholicaomnia.eu, remacle.org, and others), so for those rows the license is absent from the metadata, not stated as absent in the source.

The `TREEBANK_ANNOTATIONS` column attributes 15,197,766 tokens to `NA` (no manual treebank listed) and about 4.3 M tokens to named manual sources (Gorman 2,261,156; Pedalion 1,168,591; AGDT 472,328; PROIEL 364,200; Harrington 34,808). These are the metadata's attributions, not a per-token check.

**Redistribution implications for human review (not concluded):**

- Any Diorisis-, GLAUx-, or OGA-derived artifact that includes text would need to carry the attribution and ShareAlike terms of the base texts, subject to the per-text and per-record conflicts above.
- Whether ShareAlike obligations attach to **statistical output** (for example, frequency tables) derived from BY-SA texts depends on whether the output is a derivative or adaptation, whether counts and facts are protected at all, and on database rights in the jurisdiction. Those questions are unresolved. This file does not state that all derived frequency tables must inherit ShareAlike.
- AGDT, UD PROIEL, and UD Perseus carry non-commercial terms in at least one stated license; the commercial status of Mouseion is not relevant to a self-hosted, non-commercial deployment unless the project is later classed otherwise (not decided here).
- Redistribution of TLG content is unknown.

## Per-source detail

### 1. Perseus canonical-greekLit

- **Counts (verified by `git ls-tree` on the default branch, excluding `__cts__.xml`):** 1,612 XML files. Greek editions (`perseus-grc1`–`grc10`): 810 files, 769 distinct work keys. English (`perseus-eng1`–`eng6`): 786 files. 16 `*.1st1K-eng*`/`*.1st1K-fre*`-named files also sit in this repo; whether they are identical to First1KGreek files was not checked.
- **Tracking file:** `canonical-greekLit.tracking.json` lists 1,561 entries: 1,499 `migrated`, 62 `not migrated`.
- **Format and IDs:** TEI EpiDoc. A work-level `__cts__.xml` declares `ti:work urn="urn:cts:greekLit:tlg0011.tlg003"` and `ti:edition urn="urn:cts:greekLit:tlg0011.tlg003.perseus-grc2"`.
- **Annotation in sampled files:** the Greek file `tlg0011.tlg003.perseus-grc2.xml` (Sophocles *Ajax*, 179,099 bytes) contains no `<lemma>`, `<w>`, `<word>`, or `ana=` markup. One sample only; the repository-wide format was not inventoried.
- **Activity:** `pushed_at` 2026-10-06; HEAD `01b725d835e6e733062ffd79e0efdbae1ba06e5c`.

### 2. Open Greek and Latin First1KGreek

- **Counts (verified by `git ls-tree`):** 1,204 XML files excluding `__cts__.xml`. Filename language tags: `1st1K-grc` 969, `opp-grc` 137, `perseus-grc` 19, `1st1K-eng` 42, `1st1K-lat` 15, `opp-lat` 6, `opp-ger` 6, `1st1K-mul` 4. Greek work keys: 1,091 distinct.
- **Overlap with canonical-greekLit:** 39 Greek work keys appear in both repos (for example `tlg0011.tlg003`, `tlg0031.tlg002`).
- **Format and IDs:** TEI EpiDoc; URN form `urn:cts:greekLit:ogl0001.ogl001.1st1K-grc1`. The sampled file (`ogl0001.ogl001.1st1K-grc1.xml`, 4,747 bytes) contains no word- or lemma-level markup.
- **Meaning of `opp-*` and `1st1K-mul`:** not defined in the README and not traced. *Unknown.*

### 3. Perseus AGDT (treebank_data)

- **Versions in repo:** `v1` (36 files), `v1.6` (93), `v2.0` (45), `v2.1` (52 files total across the repo). The Greek directory of v2.1 is `v2.1/Greek/texts` (33 files, 76,478,533 bytes).
- **Texts (v2.1 README):** Aesop, Aeschylus (7 plays), Athenaeus 12–13, Diodorus 11, Herodotus 1, Hesiod (3), Homer (*Iliad*, *Odyssey*), Lysias (4 orations), Plato *Euthyphro*, Plutarch (2), Polybius 1, Pseudo-Apollodorus, Pseudo-Homer, Sophocles (5), Thucydides 1.
- **Annotation provenance (`v2.1/Greek/README.MD`):** "semi-automatically annotated … annotated manually, but many corrections have been performed automatically"; "Morphology has been annotated with the help of the Morpheus tagger." Syntax is therefore human-reviewed; morphology is Morpheus-assisted and human-corrected.
- **Sample (`v2.1/Greek/texts/tlg0540.tlg015.perseus-grc1.tb.xml`, Lysias, 74,530 bytes):** 684 `<word>` elements, 17 `<sentence>` elements. Relation labels in the sample: `ATR` 129, `OBJ` 108, `ADV` 83, `AuxX` 59, `AuxY` 35.
- **Overlap:** 31 of 32 treebank work keys match canonical-greekLit Greek keys; 9 appear in First1KGreek; `tlg0096.tlg002` does not appear in the canonical Greek file list.
- **Activity:** last commit `bf4334f0` on 2024-06-20. No newer data release found in the repo.

### 4. Diorisis Ancient Greek Corpus (automatic)

- **Record:** figshare 6187256, version 1, published 2018-05-02, modified 2023-05-30, DOI `10.6084/m9.figshare.6187256.v1`, license field `CC BY 4.0`. Authors: Alessandro Vatri, Barbara McGillivray. Reference: Vatri & McGillivray (2018), *Research Data Journal for the Humanities and Social Sciences* 3(1), 55–65, DOI `10.1163/24523666-01000013`.
- **Method, from the article:** the Crossref record of the article (read in this session; the full article was not retrieved) describes morphological enrichment followed by POS-tagger disambiguation. This is consistent with the scripts below, but the full method and lexicon provenance remain *unknown*.
- **Content (figshare description):** 820 texts, 10,206,421 words. Sources per figshare: Perseus canonical-greekLit, "The Little Sailing" digital library, Bibliotheca Augustana.
- **Package:** `Diorisis.zip`, 194,443,428 bytes, 820 `.xml` members and no README or license file (central directory, read by HTTP range requests).
- **Record header (Aelian, *Epistulae Rusticae* (003)):** `sourceDesc/ref` points to `canonical-greekLit/.../tlg0545.tlg003.perseus-grc1.xml`; `respStmt` "corpus conversion and automatic annotation"; `publicationStmt/licence/ref` `https://creativecommons.org/licenses/by-sa/3.0/us/`. IDs `tlgAuthor 0545`, `tlgId 003`. **No CTS URN** in the record.
- **Token-level sample** (same file, 2,138 word tokens):

  ```xml
  <word form="*diayu/xonti/" id="1">
    <lemma id="26366" entry="διαψύχω" POS="verb" TreeTagger="false" disambiguated="n/a">
      <analysis morph="pres part act masc/neut dat sg"/>
      <analysis morph="pres ind act 3rd pl (doric)"/>
    </lemma>
  </word>
  ```

- **Sample counts (one text):** 2,138 `<word>`, 2,138 `<lemma>`, 2,731 `<analysis>`. `TreeTagger="true"` 392; `"false"` 1,746. `disambiguated`: `1.0` 243, `0.5` 137, `0.33` 33, `0.25` 10, `0.2` 1, `n/a` 1,714.
- **Method, from `alevatri/diorisis` (commit `55ecc1d1`):** tokenization of Perseus XML, then lookup of each **form** in `grkFrm.py` (911,840 entries) to lemma IDs and morphological strings, and lemma headword/POS from `grkLemmata.py` (112,385 entries). `TT_corpus_run_and_compare.py` and `convert_corpus.py` run TreeTagger to choose among candidates; the README says probability of a choice is 1/*n* for *n* candidate lemmata. The table provenance is not stated. That the morphology strings derive from Morpheus is an inference from format, *unverified*.
- **Update channel (new in revision 2):**
  - `alevatri/diorisisupdater` (README: "script to check and download updates for Diorisis Corpus files (JSON version)") points to `https://www.crs.rm.it/diorisisCorpusupdates/latest.json`.
  - That endpoint returned HTTP 200 (48,718 bytes): an `overall` value of 1.51 and 821 per-file entries, of which 497 are `1.6` and 323 are `1.51`. This is a version manifest, not the data.
  - The data download (`requestUpdate.php`) is a form POST that carries a token embedded in the updater script. **This token was not used.** The directory listing returns HTTP 403.
  - No public Diorisis JSON release was located. The figshare record is v1 only. These are different version labels; they are not evidence that v1 and the JSON release disagree on license.
- **Derivative (new):** Zenodo 11261146 `Diorisis.duckdb` (Mark Bilby, published 2024-05-23): a DuckDB compilation of the XML files; record says the original is CC BY 4.0 and the compilation is CC BY-NC-ND 4.0. Whether the compilation's content matches the JSON release is *unknown*.
- **Repository activity:** `alevatri/diorisis` last commit 2018-10-19.
- **Overlap with canonical-greekLit (by file name only):** 812 parseable `(tlgAuthor) … (tlgWork)` pairs, 809 distinct keys. 741 match a canonical Greek work key; 102 match a First1KGreek Greek key; 31 match a treebank key; 68 match no canonical key. Not verified by content.

### 5. GLAUx (automatic; new in revision 2)

- **Identity and scope:** `alekkeersmaekers/glaux` README: "the Greek Language Automated … corpus (currently 20M tokens) … automatically annotated for morphology and syntax"; queryable at `glaux.be` (not probed). Raw XML in `xml/` (1,421 texts per `metadata.txt`).
- **Sources:** README names Perseus canonical-greekLit, First1KGreek, Wikisource (el), and other sites; "For precise details, see the file metadata.txt."
- **Counts:** 1,421 rows; 19,498,849 tokens (sum of `TOKENS`). Top sources by tokens: Perseus 9,448,627; First1K 5,078,682; el.wikisource 2,081,649; documentacatholicaomnia.eu 704,979; remacle.org 645,076.
- **Annotation (verified on a sample):**
  - Head of `xml/0001-001.xml` (text not identified in this survey): `<sentence … analysis="auto">`, `<word id form line div_book lemma postag head relation>`.
  - Random sample of 10 XML files (`random.seed(7)`, from the `xml/` listing): 215,304 `<word>` elements; `analysis="auto"` on all 8,949 sentences; 30,014 `sense=` and 33,958 `animacy=` attributes. The README says animacy and sense labels were added 10/2025.
  - This is a sample of 10 files, not the corpus. The `analysis` attribute was not checked against the manual treebank sources named in `metadata.txt`, so it is not evidence about the manual status of any particular text.
- **Repository and release:** `pushed_at` 2026-02-19; HEAD `b077d8f6` (2026-02-19, "Create CITATION.cff"). `glaux-nlp` pushed 2026-09-22. Zenodo concept `10.5281/zenodo.10948374`; v2.1 `10.5281/zenodo.17295426`, `cc-by-4.0`, 348,367,270 bytes; v1.0 `10.5281/zenodo.10948375`, `cc-by-4.0`, 323,213,508 bytes.
- **License:** README: "generally available under a CC BY-SA 4.0 license … some of the texts and/or the manual annotations may have a more restrictive license." Per-text licenses as in the table above. GitHub: no license detected for the repository. Zenodo v2.1: CC BY 4.0. These are three statements for the same v2.1 content and are unresolved.
- **Inclusion rationale:** It is the largest openly archived automatic corpus with per-token lemma, POS, and dependency relation, and it has a citable, versioned archive. It is included here despite the conflicts because it changes the earlier "only large downloadable" claim.

### 6. Opera Graeca Adnotata (OGA) (automatic; new in revision 2)

- **Identity:** Giuseppe G. A. Celano (Zenodo creator). README v0.2.0: "a multilayer corpus of Ancient Greek texts with automatically generated annotations." 1,999 base texts; 40,105,221 tokens. The site describes OGA as "the largest open-access multilayer corpus for Ancient Greek."
- **Sources (exact releases in README):** canonical-greekLit release `0.0.11376425141`; First1KGreek release `1.1.11352615003`; PatristicTextArchive release `1.1.11363682704`. Duplicates between canonical and First1K were discarded; 90 exceptions come from PTA (`urn_cts/texts/duplicates_tlg_pta.xml`).
- **Annotation (README):**
  - Tokenization, sentence segmentation, and CTS citation: rule-based.
  - Lemmatization, POS, morphological features, and dependency syntax: "outputs of (i) a parser and (ii) lemmatizer trained on 1,2M+ tokens of AGDT data," documented in arXiv `2410.12055`.
  - Composition dates: manual (README).
  - Layers listed: tokenization, sentence, lemma, morphology, syntax, dependency function, CTS.
- **Earlier version:** arXiv `2404.00739` ("Opera Graeca Adnotata: Building a 34M+ Token Multilayer Corpus for Ancient Greek"), beta v0.1.0: 1,687 works, 34M+ tokens, morphosyntax by COMBO parser trained on AGDT. This is a different release and is not treated as the same license statement.
- **Release and license (exact):** Zenodo 14206061 (v0.2.0, published 2024-11-24; DOI `10.5281/zenodo.14206061`): file `opera_graeca_adnotata_v0.2.0.zip`, 8,629,032,478 bytes, record license field `other-at`. README `## Licence`: CC BY-SA 4.0. Website footer: CC BY-SA 4.0. Repo `OperaGraecaAdnotata/OGA`: no license detected by GitHub. These are unresolved; the record field does not name a CC license.
- **Formats:** standoff PAULA XML and its successor LAULA XML; ANNIS 4 query interface. Not downloaded.
- **Derivatives found (not inspected in depth):** `mgbilby/oga_src` (README: "a simplified version and value-added adaptation"; GitHub NOASSERTION; no LICENSE file read). `Urdatorn/grc-macronizer-oga` (GPL-3.0). CATnaPS (Zenodo 15508287; CC BY-SA 4.0 field) — whether its contents derive from OGA was not verified.

### 7. Other automatic or treebank candidates checked

- **PatristicTextArchive `pta_data` (new):** 766 XML files under `data/` (including CTS metadata) and 487 under `analyzed/`. The README says `analyzed/` files are "either only automatically tokenized or automatically tokenized and linguistically analyzed; information on the used models can be found in the header of each file." Sampled `analyzed/pta0001/pta001/pta0001.pta001.pta-MsAb.xml`: 4,414 `<w xml:id>` elements, with `norm` attributes; **no `lemma=` or `pos=` attributes**; 6 `ana=` attributes; header licence CC BY-SA 4.0. So that file is tokenized only. Whether other `analyzed/` files carry lemma or POS, and which models were used, is *unknown*. Patristic Greek is not classical literature; its scope fit is a separate question.
- **Pedalion Trees (new):** `TREEBANK_LICENSE` text is CC BY-SA 4.0 (verified); the repo `LICENSE` is MIT. GLAUx's metadata lists it as a source for 1,168,591 tokens. README line 239 states the treebanks "will be" assigned CC BY-SA 4.0, which is future tense and not treated as the license.
- **AGDTmini and CATnaPS (new, Bilby, 2025):** AGDTmini (Zenodo 15508450) is described as pruned from AGDT XML and CoNLL-U into lemma@XPOS strings; stated CC BY 4.0. A derivative of BY-SA-stated material under a BY label is a licensing question for review, not a conclusion here.
- **MorphGNT / SBLGNT (new):** New Testament only. README states morphological data under CC BY-SA 3.0 and the SBLGNT text under the SBLGNT EULA. Out of scope for literary Greek; recorded for completeness.

### 8. Mirror and tooling

- **cltk/grc_text_perseus:** LICENSE.md: "Attribution-ShareAlike 3.0 United States." Last push 2023-02-01; not compared to canonical-greekLit file by file.
- **PerseusDL/morpheus:** README: "all contents of this repository are licensed under … Attribution-ShareAlike 3.0 United States License." GitHub detects no license; repository last pushed 2019-03-11. The README is an explicit statement and is recorded as such. Whether it covers code, lexicon data, or both is unresolved. The Morpheus lexicon data used by AGDT and Diorisis was not located in this repository.

### 9. TLG

- `stephanus.tlg.uci.edu/encoding/` returned HTTP 403, so TLG terms, coverage, and encoding were **not verified**.

## Annotation provenance classes

| Class | Examples | Notes |
|---|---|---|
| Text only (sampled files) | canonical-greekLit, First1KGreek | Sampled Greek files have no `<lemma>`, `<w>`, or `ana`. |
| Tokenized only (sampled file) | PTA `analyzed/` (one file) | `<w>` tokens, no lemma or POS in the sample. |
| Human-reviewed syntax, Morpheus-assisted and human-corrected morphology | AGDT v2.x | Stated in `v2.1/Greek/README.MD`. |
| Human-annotated, converted to UD | UD PROIEL, UD Perseus | Lemmas and UPOS "converted from manual" (UD README metadata). |
| Hand-built trees, one annotator (Gorman) or community (Pedalion) | Gorman, Pedalion | Stated in repo READMEs and license files. |
| Automatic: parser and lemmatizer trained on AGDT | OGA (README, arXiv `2410.12055`) | Automatic morphosyntax and lemma; dates manual. |
| Automatic: morphology and syntax (GLAUx), per-text manual sources | GLAUx | `analysis="auto"` on sampled sentences; manual sources per text in metadata. |
| Automatic: form lookup, candidate lemmas, optional TreeTagger tie-break | Diorisis | Multiple candidate `<analysis>` per token. Not human review. |
| Unknown or not traced | TLG; Diorisis lexicon provenance; GLAUx `glaux-nlp` models; PTA models | Missing annotation is not a parse. |

## Search coverage (new in revision 2)

Searches run on 2026-10-14. Coverage is listed so the absence of a source is interpretable.

- **GitHub repository search:** "GLAUx" (found the three GLAUx repositories and unrelated repositories named "glaux"); "Ancient Greek corpus automatic annotation morphology lemma" (no results returned); "Opera Graeca Adnotata" (OGA and a macronizer); "MorphGNT"; "Celano Greek corpus"; "OGA corpus ancient greek celano"; "electra-grc Ancient Greek" (no results returned).
- **Zenodo search:** "glaux" and "diorisis" (found Diorisis.duckdb, GLAUx and OGA-comparison records, and unrelated records).
- **Crossref:** Diorisis article DOI and queries on Vatri/McGillivray and lemmatization (found the Diorisis article; GASC, 2019, and a lemmatization article of 2020 not read).
- **arXiv:** `2404.00739` (OGA beta abstract), `2410.12055` (parser and lemmatizer abstract).
- **Web search tool:** GLAUx query returned no results; Diorisis query returned no results.
- **Not searched in depth:** `Pedalion` project pages; `Tesserae`; `Logeion`; `papyri.info`; Papyri and inscriptions (out of literary scope); Wikisource bulk exports; `CLTK` corpora beyond the Perseus mirror; `Ancient Greek BERT` models; the Perseus `Morpheus` lexicon as data. These may hold further candidates and are listed as unsearched.

## Reproducible commands

Scratch was `~/.cache/scratch/ancient-greek-sources-2022-r2` (removed after publication). Commands are the ones that produced the cited facts.

```sh
# Repository metadata (GitHub REST)
gh api repos/alekkeersmaekers/glaux --jq '[.license.spdx_id,.pushed_at]|@tsv'
gh api repos/OperaGraecaAdnotata/OGA/readme -H 'Accept: application/vnd.github.raw'
gh api repos/PatristicTextArchive/pta_data/readme -H 'Accept: application/vnd.github.raw'

# Zenodo records (JSON API)
curl -sSL https://zenodo.org/api/records/17295426   # GLAUx v2.1 license/size
curl -sSL https://zenodo.org/api/records/14206061   # OGA v0.2.0 license field "other-at"
curl -sSL https://zenodo.org/api/records/11261146   # Diorisis.duckdb derivative

# GLAUx metadata (blobless clone; metadata.txt is tab-separated)
git clone --filter=blob:none --no-checkout --depth 1 https://github.com/alekkeersmaekers/glaux.git
git -C glaux show HEAD:metadata.txt > glaux-metadata.tsv
# then sum TOKENS and count SOURCE_LICENSE with csv.DictReader (python3 -I)

# Diorisis update manifest (manifest only; the data POST was not sent)
curl -sSL https://www.crs.rm.it/diorisisCorpusupdates/latest.json
```

`ls-tree -l` on the blobless GLAUx clone timed out (it fetches blobs). Use `ls-tree -r --name-only` instead.

## Unknowns and limitations

1. **TLG terms and coverage:** not retrieved (HTTP 403).
2. **Diorisis full method and lexicon provenance:** the article's full text was not retrieved; the origin of `grkFrm.py`/`grkLemmata.py` is not stated; the Morpheus link is an inference.
3. **Diorisis newer data:** the JSON version's data channel is token-gated and was not used; no public JSON release was located; its license is *unknown*. The derivative DuckDB (Zenodo 11261146) is not known to match any version.
4. **GLAUx and OGA annotation quality:** no accuracy evaluation was found for either; the sample of GLAUx files shows `analysis="auto"` but says nothing about correctness.
5. **GLAUx per-file licenses:** only `metadata.txt` was summed; the TEI headers of individual GLAUx texts were not audited.
6. **OGA record vs. README:** the Zenodo `other-at` field and the README CC BY-SA 4.0 statement were both read; which governs the archive is not decided.
7. **PTA annotation layers:** one sampled `analyzed/` file, tokenized only. Other files and models were not checked.
8. **Perseus copyright statement:** the canonical README's "overall copyright" text is not reconciled with the CC BY-SA 4.0 `license.md`.
9. **Per-file license metadata** in canonical-greekLit, First1KGreek, and PTA was not audited beyond the sampled headers.
10. **Frequency-bias facts:** the effect of duplicate editions on counts was not quantified beyond work-level key overlap.
11. **No Mouseion code was read or run**; this file is upstream evidence only.

## Recommendation inputs

These are inputs to later human decisions, not decisions. Each is a question for review, not a prohibition.

1. **Text-only sources with explicit BY-SA 4.0 in-repo license:** canonical-greekLit and First1KGreek. Most maintained, with stable CTS URNs; no lexical annotation in sampled files, so Mouseion would need its own annotation or another source.
2. **Automatic corpora requiring license review before use:**
   - **GLAUx** (about 19.5 M tokens; automatic morphology and syntax; per-text licenses; README CC BY-SA 4.0 vs. Zenodo CC BY 4.0). Largest openly archived automatic corpus with lemma, POS, and dependency relations.
   - **OGA** (about 40.1 M tokens; automatic morphosyntax and lemma from an AGDT-trained parser; README CC BY-SA 4.0 vs. Zenodo `other-at`; 8.6 GB archive).
   - **Diorisis** (about 10.2 M words; automatic lookup with TreeTagger tie-break; figshare CC BY 4.0 vs. per-text CC BY-SA 3.0 US; derivative DuckDB is CC BY-NC-ND 4.0; no public newer JSON release found).
   - Each of these has an unresolved license statement. None is excluded on that basis alone.
3. **Human-reviewed small set:** AGDT v2.1 Greek (about 33 texts; syntax human-reviewed; morphology Morpheus-assisted; root README BY-SA 3.0 US vs. v1 docs BY-NC-SA 2.5; no commits since 2024-06-20). Pedalion Trees (CC BY-SA 4.0 `TREEBANK_LICENSE`; MIT repo `LICENSE`) and Gorman (explicit CC BY-SA 4.0 `TREEBANK LICENSE`) are further human-annotated options.
   - **Gorman is not excluded for licensing.** Its `TREEBANK LICENSE` is explicit. Its status is a single-annotator, dormant (2022-08-15) collection with unmeasured size.
4. **Non-commercial alternatives:** UD PROIEL (CC BY-NC-SA 3.0) and UD Perseus (CC BY-NC-SA 2.5), maintained and human-converted.
5. **Morpheus:** explicit README CC BY-SA 3.0 US statement; scope (code vs. lexicon data) to be read by a human.
6. **Overlap is material:** Diorisis overlaps canonical-greekLit by work key (741 of 809 keys, by file name); OGA is built from canonical-greekLit and First1KGreek plus PTA. Any combined source needs a CTS work-URN dedupe key before frequency aggregation.
7. **Not usable as evidence yet:** TLG (unverified), PTA (tokenized sample only), `cltk` mirror (dormant mirror).

## Issues revealed

**Precise questions (answerable by a narrow further investigation):**

- Does any Diorisis release (figshare, Zenodo, or the crs.rm.it JSON channel) carry a license statement that reconciles CC BY 4.0 with the per-text CC BY-SA 3.0 US, and what license does the JSON channel carry? (The token-gated download needs an authorized route.)
- What is the full Diorisis method and lexicon provenance? (Brill article full text.)
- Does GLAUx v2.1's Zenodo `cc-by-4.0` apply to the whole archive, or only to the README-described CC BY-SA 4.0 portions? (Contact or a per-text audit of `metadata.txt` plus TEI headers.)
- Does OGA's Zenodo `other-at` license field match the README's CC BY-SA 4.0 for the same v0.2.0 archive?
- Do PTA `analyzed/` files other than the sample carry lemma or POS, and which models are named in their headers?
- Do canonical-greekLit and First1KGreek TEI headers carry per-file license statements that match the repo-level license?
- What do First1KGreek `opp-grc` and `1st1K-mul` mean?
- Does Morpheus's README license cover the lexicon data, the code, or both, and where is the lexicon data published?

**Fog (not precisely answerable from current evidence):**

- The legal weight of Perseus's "overall copyright" statement alongside CC BY-SA 4.0.
- Whether statistical output derived from BY-SA texts inherits ShareAlike (depends on derivative status, protectability of counts, and database rights). Legal question for review.
- Whether TLG-licensed text can be used in a self-hosted app under any terms (TLG access not retrieved).
- Annotation accuracy of the automatic corpora (Diorisis, GLAUx, OGA): no independent evaluation found in this survey.

## Publication blockers

None blocks publication of this evidence. Unresolved items are recorded above as unresolved (Diorisis license and newer data, GLAUx and OGA license conflicts, TLG access), not as blockers to the record.
