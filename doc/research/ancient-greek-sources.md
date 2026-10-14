# Ancient Greek text repositories and machine-analyzed corpora

**Checked:** 2026-10-14 (upstream state as read on this date; repository `pushed_at` and commit dates cited below)

**Ticket:** [#2022](https://github.com/justin-hayes/mouseion/issues/2022), child of [#2021](https://github.com/justin-hayes/mouseion/issues/2021).

**Question:** Which maintained Ancient Greek text repositories and large machine-analyzed corpora can Mouseion responsibly use today, and under what acquisition and redistribution terms?

**Scope limits:** This file records evidence only. It does not choose Mouseion architecture, the MVP, or any acquisition policy. Items marked *unknown* were not verified.

## Summary

- **Open-licensed TEI text (no linguistic annotation):** Perseus `canonical-greekLit` and Open Greek and Latin `First1KGreek` are both published under CC BY-SA 4.0 in their own repositories. Both are actively maintained (last pushes 2026-10-06 and 2026-09-30). Both are CTS-identified and contain **text and CTS metadata only**: no lemmas, POS, or morphology.
- **Human-reviewed syntax + morphology (small):** Perseus Ancient Greek Dependency Treebank (AGDT) `treebank_data` v2.1 has 33 Greek text files covering 32 works, with manual syntax and Morpheus-assisted morphology. The repository's root README says CC BY-SA 3.0 US, but its own v1 documentation says CC BY-NC-SA 2.5. Repository activity stopped at 2024-06-20.
- **Automatic corpus (large):** The Diorisis Ancient Greek Corpus (Vatri & McGillivray) has 820 TEI XML texts, 10,206,421 words per its figshare record. Its per-token lemma/analysis annotation is **automatic dictionary lookup** with optional TreeTagger tie-breaking, not human review. Its figshare record says CC BY 4.0, but the license inside every text says CC BY-SA 3.0 US, and its source texts are largely Perseus texts (CC BY-SA). The annotation repo has no license file and its last commit is 2018-10-19. Diorisis is the only large downloadable per-token machine-analyzed Greek corpus found in this survey; the license conflict must be resolved before any use.
- **Human-annotated alternatives with restrictive licenses:** UD Ancient Greek PROIEL (CC BY-NC-SA 3.0; New Testament and Herodotus excerpts) and UD Ancient Greek Perseus (CC BY-NC-SA 2.5; AGDT-derived) are actively maintained but **non-commercial**. Gorman's hand-built `Greek-Dependency-Trees` is CC BY-SA 4.0 with no GitHub license metadata and no commits since 2022-08-15.
- **Mirror and tooling:** `cltk/grc_text_perseus` mirrors Perseus Greek files (CC BY-SA 3.0 US per its LICENSE.md) but was last pushed 2023-02-01. `PerseusDL/morpheus` is the Morpheus parser code, with no license metadata and a README that restates the Perseus-content license.
- **Not verified:** the TLG (Thesaurus Linguae Graecae) terms could not be retrieved (HTTP 403 on `stephanus.tlg.uci.edu`). It is treated as unknown and restricted until checked.

## Source inventory

Sizes are from GitHub's `size` field (KB) or the figshare `size` field (bytes). "Files" counts XML members excluding `__cts__.xml` CTS metadata. License rows give the **stated** license, and where the same layer carries a different license elsewhere, the conflict is listed separately in the licensing section.

| Source | Layer | Stated license (where) | Coverage (verified) | Format / IDs | Last activity | Archival vs. active |
|---|---|---|---|---|---|---|
| [PerseusDL/canonical-greekLit](https://github.com/PerseusDL/canonical-greekLit) @ `01b725d8` | Text + CTS metadata | CC BY-SA 4.0 (`license.md`; README; GitHub `CC-BY-SA-4.0`) | 1,612 XML files: 810 `perseus-grc*` (Greek), 786 `perseus-eng*` (English), 16 `1st1K-eng*` copies | TEI EpiDoc; CTS URN `urn:cts:greekLit:tlgNNNN.tlgNNN.perseus-grcN`; `__cts__.xml` per work; 769 distinct Greek work keys | pushed 2026-10-06; HEAD commit 2026-10-06 | Active |
| [OpenGreekAndLatin/First1KGreek](https://github.com/OpenGreekAndLatin/First1KGreek) @ `03776b39` | Text + CTS metadata | CC BY-SA 4.0 (`license.md`; GitHub `CC-BY-SA-4.0`) | 1,204 XML files: 1,126 with `grc` in the filename (969 `1st1K-grc`, 137 `opp-grc`, 19 `perseus-grc`); 1,091 distinct Greek work keys; also eng/lat/ger | TEI EpiDoc; CTS URN `urn:cts:greekLit:ogl0001.ogl001.1st1K-grc1`; `__cts__.xml` | pushed 2026-09-30; HEAD commit 2026-09-30 | Active (Zenodo DOI in README: 10.5281/zenodo.596723) |
| [PerseusDL/treebank_data](https://github.com/PerseusDL/treebank_data) @ `bf4334f0` (AGDT) | Syntax + morphology (human-reviewed) | README: CC BY-SA 3.0 US; `v1/greek/docs/README.txt`: CC BY-NC-SA 2.5; GitHub license `null` | `v2.1/Greek/texts`: 33 files, 32 distinct Greek work keys, 76.5 MB; `v2.0`/`v1.6` have additional files | TB XML: `<treebank version cts>`, `<word id form lemma postag relation head>` per sentence; CTS `urn:cts:greekLit:tlgNNNN.tlgNNN.perseus-grcN.tb` | last commit 2024-06-20 | Archival (data stable; no commits since 2024) |
| [alevatri/diorisis](https://github.com/alevatri/diorisis) @ `55ecc1d1` (annotation scripts) | Scripts, lexicon tables | none in repo (no LICENSE file; README silent) | `grkLemmata.py`: 112,385 lemma entries; `grkFrm.py`: 911,840 form entries | Python dictionaries | last commit 2018-10-19; `pushed_at` 2024-06-27 | Archival |
| [Diorisis Ancient Greek Corpus](https://figshare.com/articles/dataset/Diorisis_Ancient_Greek_Corpus/6187256) (figshare 6187256, v1) | Text + automatic per-token analysis | figshare: CC BY 4.0; each text's `teiHeader` licence: CC BY-SA 3.0 US | 820 XML files; 10,206,421 words (figshare); Homer to 5th c. AD; sources: Perseus canonical-greekLit, "The Little Sailing", Bibliotheca Augustana | modified 2023-05-30 | Archival (single version v1) |
| [figshare 7229162](https://figshare.com/articles/dataset/Diorisis_Corpus_-_Preprocessed_files/7229162) (preprocessed Diorisis input) | Pre-annotation tokenized files | CC BY 4.0 (figshare) | `annoCorpora.zip`, 59,893,269 bytes | Tokenized Perseus-like XML | modified 2023-05-31 | Archival |
| [UniversalDependencies/UD_Ancient_Greek-PROIEL](https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL) | Syntax + morphology (converted from manual; UD) | CC BY-NC-SA 3.0 (UD README metadata; GitHub `NOASSERTION`) | New Testament plus selections from Herodotus (UD README) | CoNLL-U; `UD 2.12` changelog | pushed 2026-10-05 | Active |
| [UniversalDependencies/UD_Ancient_Greek-Perseus](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus) | Syntax + morphology (converted from manual; UD) | CC BY-NC-SA 2.5 (UD README metadata; GitHub `NOASSERTION`) | Fiction (AGDT-derived texts) | CoNLL-U; changelog to v2.11 (2022-11-15) | pushed 2026-10-05 | Active |
| [vgorman1/Greek-Dependency-Trees](https://github.com/vgorman1/Greek-Dependency-Trees) | Syntax (one annotator; some pre-parsed then hand-corrected) | `TREEBANK LICENSE`: CC BY-SA 4.0 (GitHub license `null`) | Prose trees (e.g., Demosthenes); count not measured | Arethusa/Perseids XML | last push 2022-08-15 | Dormant |
| [cltk/grc_text_perseus](https://github.com/cltk/grc_text_perseus) | Mirror of Perseus Greek text | CC BY-SA 3.0 US (`LICENSE.md`; README) | Mirror of Perseus files (not compared file by file) | Perseus-derived XML | pushed 2023-02-01 | Dormant mirror |
| [PerseusDL/morpheus](https://github.com/PerseusDL/morpheus) | Morphological analyzer **code** | None in GitHub metadata; README restates the Perseus-content license (CC BY-SA 3.0 US) | n/a (software) | Source code | pushed 2019-03-11 | Dormant |
| TLG (Thesaurus Linguae Graecae) | Text + annotation (commercial) | **unknown**: `stephanus.tlg.uci.edu` returned HTTP 403 | unknown | unknown | unknown | unknown |

## Licensing by layer

Layers are kept separate: text, linguistic annotation, metadata, and derived data. Nothing below is a legal conclusion; each row is what the upstream repository or record states.

| Layer | Source | Stated | Conflict or gap |
|---|---|---|---|
| Text | canonical-greekLit | CC BY-SA 4.0 (repo `license.md`) | The repo README also says Tufts holds "overall copyright" and that materials are "provided for the personal use of students, scholars, and the public", with copyright "varying" by component, and asks readers to contact the project about specific components. Not reconciled. |
| Text | First1KGreek | CC BY-SA 4.0 (repo `license.md`) | Per-file license statements in the TEI headers were not checked. |
| Text | AGDT (treebank_data) | README: CC BY-SA 3.0 US; "All materials in the AGLDT to which third-parties do not hold copyright are copyright of … Tufts" | v1 docs say CC BY-NC-SA 2.5. Conflicting for the same data lineage. |
| Annotation | AGDT v2.x | Repo root README (BY-SA 3.0 US) | Same as above; v2.x README gives no license of its own. |
| Annotation | UD PROIEL | CC BY-NC-SA 3.0 | Non-commercial; ShareAlike. |
| Annotation | UD Perseus | CC BY-NC-SA 2.5 | Non-commercial; ShareAlike. Lineage of AGDT (see above). |
| Annotation | Gorman trees | CC BY-SA 4.0 (`TREEBANK LICENSE`) | No GitHub license metadata; single annotator. |
| Annotation | Diorisis (per-token lemmas/analyses) | figshare: CC BY 4.0 (figshare `files`/record); per-text `licence`: CC BY-SA 3.0 US | **Conflict** inside the same record. The annotation scripts repo has no license at all. |
| Metadata | Diorisis `teiHeader` | CC BY-SA 3.0 US (in each text) | Same as above. |
| Metadata | canonical-greekLit / First1KGreek CTS `__cts__.xml` | Inherits repo license (CC BY-SA 4.0) | Not separately stated. |
| Derived | Diorisis derived from Perseus | Perseus texts are CC BY-SA (4.0 per current repo; 3.0 US per cltk and the Diorisis text headers) | ShareAlike obligations on a derivative carrying a CC BY 4.0 label are unresolved here; not a legal opinion. |
| Derived | cltk/grc_text_perseus | CC BY-SA 3.0 US | Older mirror of Perseus content. |
| Software | PerseusDL/morpheus | None in metadata | Code license unknown. |

**Redistribution implications noted for human review (not concluded):** Any Diorisis-derived artifact (including a Mouseion-generated frequency table) would have to carry the attribution and ShareAlike terms of the Perseus base text; the CC BY 4.0 claim does not remove them. AGDT and UD Perseus/PROIEL are non-commercial in at least one stated license. Redistribution of the TLG is unknown.

## Per-source detail

### 1. Perseus canonical-greekLit

- **Identity and scope:** Repo description "XML Canonical resources for Greek Literature". The root `README.md` states it is the base repository for text and annotation data published in the Perseus Digital Library.
- **Counts (verified by `git ls-tree` on the default branch, excluding `__cts__.xml`):** 1,612 XML files. Greek editions (`perseus-grc1`–`grc10`): 810 files, 769 distinct work keys (`tlgNNNN.tlgNNN`). English (`perseus-eng1`–`eng6`): 786 files. 16 `*.1st1K-eng*`/`*.1st1K-fre*`-named files, using First1KGreek naming, also sit in this repo; whether they are identical to First1KGreek files was not checked.
- **Tracking file:** `canonical-greekLit.tracking.json` lists 1,561 entries: 1,499 `migrated`, 62 `not migrated`.
- **Format and IDs:** TEI EpiDoc. A work-level `__cts__.xml` declares `ti:work urn="urn:cts:greekLit:tlg0011.tlg003"` and `ti:edition urn="urn:cts:greekLit:tlg0011.tlg003.perseus-grc2"`.
- **Annotation completeness:** The sampled Greek file (`tlg0011.tlg003.perseus-grc2.xml`, Sophocles *Ajax*, 179,099 bytes) contains **no** lemma, word, or morphological markup (zero matches for `<lemma`, `<w `, `<word`, `ana=`). It is text only.
- **Activity:** `pushed_at` 2026-10-06; HEAD commit `01b725d835e6e733062ffd79e0efdbae1ba06e5c` (2026-10-06, "Merge pull request #2003 … typos_current").

### 2. Open Greek and Latin First1KGreek

- **Identity and scope:** "XML files for the works in the First Thousand Years of Greek Project." Zenodo DOI 10.5281/zenodo.596723 in README.
- **Counts (verified by `git ls-tree`):** 1,204 XML files excluding `__cts__.xml`. Filename language tags: `1st1K-grc` 969, `opp-grc` 137, `perseus-grc` 19, `1st1K-eng` 42, `1st1K-lat` 15, `opp-lat` 6, `opp-ger` 6, `1st1K-mul` 4. Greek work keys: 1,091 distinct (`ogl`, `ggm`, `sto` prefixes account for 6 keys; the rest are `tlg`).
- **Overlap with canonical-greekLit:** 39 Greek work keys appear in both repos (for example `tlg0011.tlg003`, `tlg0031.tlg002`, `tlg0057.tlg010`, `tlg0081.tlg006`).
- **Format and IDs:** TEI EpiDoc; URN form `urn:cts:greekLit:ogl0001.ogl001.1st1K-grc1`. The sampled file (`ogl0001.ogl001.1st1K-grc1.xml`, 4,747 bytes) contains no word- or lemma-level markup.
- **Meaning of `opp-*`:** the tag is not defined in the README and was not traced. *Unknown.*
- **Activity:** `pushed_at` 2026-09-30; HEAD `03776b39f4047c5cff06f5296fae4b2bae4b08fb` (2026-09-30, merge of PR #2890).

### 3. Perseus Ancient Greek Dependency Treebank (treebank_data)

- **Versions in repo:** `v1` (36 files), `v1.6` (93), `v2.0` (45), `v2.1` (52 files total across the repo). The Greek-only directory of `v2.1` is `v2.1/Greek/texts` (33 files, 76,478,533 bytes), with `TAGSETS.xml` and a README.
- **Texts (v2.1 README):** Aesop, Aeschylus (7 plays), Athenaeus 12–13, Diodorus 11, Herodotus 1, Hesiod (3), Homer (*Iliad*, *Odyssey*), Lysias (4 orations), Plato *Euthyphro*, Plutarch (2), Polybius 1, Pseudo-Apollodorus 1.1.1–1.4.1, Pseudo-Homer, Sophocles (5), Thucydides 1.
- **Annotation provenance (stated in `v2.1/Greek/README.MD`):** "semi-automatically annotated … annotated manually, but many corrections have been performed automatically"; "Morphology has been annotated with the help of the Morpheus tagger." Guidelines: AGDT 1.1 (GSAAGDT) and AGDT 2.0 (GAGDT). Annotators are named in the README. So syntax is human-reviewed, while morphology is Morpheus-assisted and human-corrected.
- **Sample (`v2.1/Greek/texts/tlg0540.tlg015.perseus-grc1.tb.xml`, 74,530 bytes; Lysias):** 684 `<word>` elements, 17 `<sentence>` elements. Example records:

  ```xml
  <treebank version="2.1" xml:lang="grc" cts="urn:cts:greekLit:tlg0540.tlg015.perseus-grc1.tb">
  <sentence id="1" document_id="urn:cts:greekLit:tlg0540.tlg015.perseus-grc1" subdoc="1-12">
  <word id="1" form="ἐγὼ" lemma="ἐγώ" postag="p1s---mn-" relation="SBJ" head="10"/>
  <word id="2" form="μέν" lemma="μέν" postag="d--------" relation="AuxY" head="15"/>
  ```

  Relation labels in the sample: `ATR` 129, `OBJ` 108, `ADV` 83, `AuxX` 59, `AuxY` 35.
- **Overlap:** The 32 treebank work keys include 31 that match canonical-greekLit Greek work keys, and 9 that appear in First1KGreek. One treebank key (`tlg0096.tlg002`) does not appear in the canonical Greek file list.
- **Activity:** last commit `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` on 2024-06-20 ("Update Greek_guidelines_2.5.md"). No newer data release found in the repo.

### 4. Diorisis Ancient Greek Corpus (automatic)

- **Record:** figshare 6187256, version 1, published 2018-05-02, modified 2023-05-30, DOI `10.6084/m9.figshare.6187256.v1`, license field `CC BY 4.0`. Authors: Alessandro Vatri, Barbara McGillivray (ORCID listed). Reference: Brill, *Research Data Journal for the Humanities and Social Sciences* 3(1), 2018 (the Brill page could not be fetched in this session; *unknown* whether it details the method beyond the scripts).
- **Content (figshare description):** 820 texts, 10,206,421 words, "between the beginnings of the AG literary tradition (Homer) and the fifth century AD". Sources: Perseus canonical-greekLit, "The Little Sailing" digital library, Bibliotheca Augustana.
- **Package:** one archive `Diorisis.zip`, 194,443,428 bytes, containing **820 `.xml` members and no README or license file** (verified from the central directory). Uncompressed total 2,517,470,203 bytes.
- **Record header (Aelian, *Epistulae Rusticae* (003)):**
  - `sourceDesc/ref target` points to `https://github.com/PerseusDL/canonical-greekLit/tree/master/data/tlg0545/tlg003/tlg0545.tlg003.perseus-grc1.xml`.
  - `respStmt resp`: "corpus conversion and automatic annotation" (Vatri); "principal investigator" (McGillivray).
  - `publicationStmt/licence/ref`: `https://creativecommons.org/licenses/by-sa/3.0/us/`.
  - IDs: `tlgAuthor 0545`, `tlgId 003`. **No CTS URN** in the record.
- **Token-level sample** (`sentence id="1"`, same file, 2,138 word tokens):

  ```xml
  <word form="*diayu/xonti/" id="1">
    <lemma id="26366" entry="διαψύχω" POS="verb" TreeTagger="false" disambiguated="n/a">
      <analysis morph="pres part act masc/neut dat sg"/>
      <analysis morph="pres ind act 3rd pl (doric)"/>
    </lemma>
  </word>
  ```

- **Sample statistics** (`grep` counts on the one text): 2,138 `<word>` tokens; 2,138 `<lemma>` elements; 2,731 `<analysis>` elements. `TreeTagger="true"` on 392 lemma elements, `"false"` on 1,746. `disambiguated`: `1.0` 243, `0.5` 137, `0.33` 33, `0.25` 10, `0.2` 1, `n/a` 1,714. POS counts (top): verb 426, noun 287, pronoun 253, adjective 244, conjunction 243, article 213, particle 205.
- **Method (from `alevatri/diorisis` README and scripts, commit `55ecc1d1`):**
  1. `tokenizePerseus.py` tokenizes Perseus XML; `tokenizeConverted.py` tokenizes other collections.
  2. `corporaParser.py` annotates each token by looking up its **form** in `grkFrm.py` (a table of 911,840 beta-code-keyed form entries, each giving lemma IDs and Morpheus-style morphological strings) and the lemma-ID table `grkLemmata.py` (112,385 entries giving headword and POS). The tables' provenance is not stated in the repo. The morphology strings (e.g., `fem dat pl (doric)`) look like Morpheus output; that is an inference from format and is *unverified*.
  3. `TT_corpus_run_and_compare.py` and `convert_corpus.py` run TreeTagger to choose among candidate lemmas. README: "For each token, TreeTagger may select a POS represented by *n* lemmata (probability of disambiguation: 1/*n*)." This is how the `disambiguated` value is read; the *n/a* value means no TreeTagger tie-break was recorded (inferred from the README).
  4. The output is therefore a **multi-candidate, lexicon-based analysis**, not a per-token human decision. The corpus is not human-reviewed (the figshare record says "automatic annotation").
- **Preprocessed inputs:** figshare 7229162 ("Diorisis Corpus - Preprocessed files"), `annoCorpora.zip`, 59,893,269 bytes, CC BY 4.0, modified 2023-05-31.
- **Repository activity:** `alevatri/diorisis` last commit 2018-10-19 (`pushed_at` 2024-06-27); a companion `alevatri/diorisisupdater` (last push 2021-04-27) checks for "updates for Diorisis Corpus files (JSON version)". The JSON version is not in the figshare record (v1 only). *Unknown* whether a newer version exists.
- **Overlap with canonical-greekLit:** Parsing the 820 file names gives 812 parseable `(tlgAuthor) … (tlgWork)` pairs, 809 distinct keys (8 Plutarch files with letter-suffixed numbers were not matched by the regex). Of these, **741** match a canonical Greek work key; 102 match a First1KGreek Greek key; 31 match a treebank key. 68 keys match no canonical Greek key (consistent with the Little Sailing / Augustana sources, not verified file by file).

### 5. Alternatives with human annotation and restrictive licenses

- **UD_Ancient_Greek-PROIEL** (`pushed_at` 2026-10-05): converted from the PROIEL treebank; contains most of the New Testament plus selections from Herodotus. "Lemmas: converted from manual"; "UPOS: converted from manual". License CC BY-NC-SA 3.0 (UD README metadata; GitHub `NOASSERTION`).
- **UD_Ancient_Greek-Perseus** (`pushed_at` 2026-10-05): converted from the AGDT (fiction). "Lemmas: converted from manual"; contributors Celano and Zeman. License CC BY-NC-SA 2.5 (UD README metadata; GitHub `NOASSERTION`).
- **Gorman Greek-Dependency-Trees**: a hand-built syntax collection by one annotator, "constantly edited … and expanded". Trees from 2020 on were pre-parsed at the Pedalion Project and hand-corrected. `TREEBANK LICENSE` is CC BY-SA 4.0; GitHub reports no license. Last push 2022-08-15. Count of trees and tokens not measured.

### 6. Mirror and tooling

- **cltk/grc_text_perseus:** "Collected Greek files made available by the Perseus Project". LICENSE.md: "Attribution-ShareAlike 3.0 United States". Last push 2023-02-01; not compared to canonical-greekLit file by file.
- **PerseusDL/morpheus:** "Morpheus parser code." README: "all contents of this repository are licensed under … Attribution-ShareAlike 3.0 United States License." GitHub metadata has no license. Last push 2019-03-11. The Morpheus morphological **data** (lexicon) used by the AGDT and Diorisis was not located in an upstream repo this session (*unknown*).

### 7. TLG

- `stephanus.tlg.uci.edu/encoding/` returned HTTP 403 to the web fetcher, so the TLG terms, coverage, and encoding were **not verified**. The TLG is generally understood to be licensed rather than openly published; that is *not* confirmed here.

## Annotation provenance classes

| Class | Examples found | Notes |
|---|---|---|
| Text only, no linguistic annotation | canonical-greekLit, First1KGreek | Sampled Greek files have no `<lemma>`, `<w>`, or `ana`. |
| Human-annotated (syntax) with Morpheus-assisted morphology, human-corrected | AGDT `v2.x` | Stated in `v2.1/Greek/README.MD`. |
| Human-annotated, converted to UD | UD PROIEL, UD Perseus | Lemmas and UPOS "converted from manual" (UD README metadata). |
| Hand-built, single annotator, pre-parsed then hand-corrected (post-2020 trees) | Gorman | Stated in repo README. |
| Automatic: form-lookup with candidate lemmas and analyses; optional TreeTagger tie-break | Diorisis | Multiple candidate `<analysis>` per token; `disambiguated` is a 1/n probability in the README's terms. Not equal to human review. |
| Unknown provenance | TLG (not checked); Diorisis `grkFrm`/`grkLemmata` table origin (not stated) | Missing annotation is not a parse. |

## Reproducible commands

These are the commands used. Scratch was `~/.cache/scratch/ancient-greek-sources-2022` (removed after publication).

```sh
# Repository metadata (license, pushed_at), GitHub REST
gh api repos/PerseusDL/canonical-greekLit --jq '{license:.license.spdx_id,pushed_at}'
gh api repos/OpenGreekAndLatin/First1KGreek --jq '{license:.license.spdx_id,pushed_at}'
gh api repos/PerseusDL/treebank_data --jq '{license:.license,pushed_at}'
gh api repos/alevatri/diorisis --jq '{license:.license,pushed_at}'

# Blobless clones (trees only; blobs fetched on demand)
git clone --filter=blob:none --no-checkout --depth 1 https://github.com/PerseusDL/canonical-greekLit.git
git clone --filter=blob:none --no-checkout --depth 1 https://github.com/OpenGreekAndLatin/First1KGreek.git
git clone --filter=blob:none --no-checkout --depth 1 https://github.com/PerseusDL/treebank_data.git
git -C canonical-greekLit ls-tree -r --name-only HEAD data | grep -E '\.xml$' | grep -v __cts__ | grep -c perseus-grc
git -C treebank_data ls-tree -r -l HEAD v2.1/Greek/texts | wc -l
git -C canonical-greekLit show HEAD:license.md | head -3
git -C treebank_data show HEAD:v1/greek/docs/README.txt | sed -n '13,18p'

# Diorisis record and license (figshare API)
curl -sSL https://api.figshare.com/v2/articles/6187256 | python3 -m json.tool | less
```

Remote-zip probe (listing and one member without downloading all 194 MB): resolve the figshare `ndownloader` 302 to its S3 URL (its signature expires after about 10 seconds), then issue HTTP `Range` GETs and read the central directory with `zipfile`. The probe transferred about 130 KB in total. The Diorisis member listing and the sampled file are reproducible this way.

## Unknowns and limitations

1. **TLG terms and coverage:** not retrieved (HTTP 403).
2. **Diorisis method and provenance of the lexicon tables:** the Brill article was not retrieved; the origin of `grkFrm.py`/`grkLemmata.py` is not stated in the repo; the Morpheus link is an inference.
3. **Diorisis version:** only figshare v1 was found; `alevatri/diorisisupdater` mentions a JSON update channel that was not located.
4. **Diorisis work-level comparison:** based on file names only, not on text content or edition identity.
5. **Token counts for AGDT v2.1 and the canonical/First1K corpora:** not measured (only the file, byte, and work counts above). Downloading all 76 MB of AGDT Greek texts was not needed for the licence question; a full token count is open.
6. **Morphological data in the AGDT:** Morpheus-assisted but not separately licensed upstream; the Morpheus data source was not located.
7. **Perseus copyright statement:** the canonical README's "overall copyright" text is not reconciled with its CC BY-SA 4.0 `license.md`. Interpreting it is a legal question and is not decided here.
8. **Per-file license metadata:** only repository-level and per-record licenses were checked. Individual TEI headers in canonical-greekLit and First1KGreek were not audited.
9. **Diorisis derivation:** the claim that Diorisis texts all come from Perseus, Little Sailing, and Augustana is the figshare description and was not verified for each file.
10. **Frequency-bias facts:** no duplicate-edition analysis beyond work-level key overlap was done, so the effect of duplicates on frequency counts is not quantified.
11. **No runtime test:** no Mouseion code was read or run; this file is upstream evidence only.

## Recommendation inputs

These are inputs to later human decisions, not decisions.

1. **Text-only, openly licensed sources:** canonical-greekLit and First1KGreek are the most maintained and most clearly licensed (CC BY-SA 4.0 in-repo), with stable CTS URNs. They carry no lexical annotation, so Mouseion would need its own lemmatization or a separate annotation source.
2. **Small human-annotated set:** AGDT v2.1 Greek is the only openly published human-reviewed syntax set found with CTS URNs. Its license is contradictory (BY-SA 3.0 US vs. BY-NC-SA 2.5 in v1 docs), and it has been dormant since 2024-06-20.
3. **Large automatic corpus:** Diorisis is the only downloadable large per-token analysis found. It has the most coverage (820 texts; 10.2 M words per figshare) and its per-token candidate and disambiguation data are explicit. Its licensing is internally contradictory, its repo has no license, and its last update was 2023 (figshare) or 2018 (scripts). Treat any use as requiring license resolution and provenance labelling, not as human annotation.
4. **Restrictive alternatives:** UD PROIEL and UD Perseus are maintained and human-converted but non-commercial by their stated licenses.
5. **Not usable as evidence yet:** TLG (unverified), Gorman (single-annotator, dormant, no GitHub license), cltk mirror (dormant, mirror of canonical).
6. **Overlap is material:** 741 of Diorisis's 809 distinct works match canonical-greekLit Greek works, and 39 works are present in both canonical-greekLit and First1KGreek. Any combined source needs a dedupe key (CTS work URN or tlgAuthor/tlgWork) before frequency aggregation.

## Issues revealed

**Precise questions (answerable by a further, narrow investigation):**

- Does the Diorisis figshare record or a later release include a LICENSE file or updated `licence` value that reconciles CC BY 4.0 with the per-text CC BY-SA 3.0 US?
- What is the full Diorisis annotation method and lexicon provenance (Brill article; authors)?
- Do the canonical-greekLit and First1KGreek TEI headers carry any per-file license statements, and do they match the repo-level license?
- What is the total Greek token count for AGDT v2.1 and for canonical/First1K Greek editions (requires fetching all Greek blobs)?
- What do First1KGreek `opp-grc` and `1st1K-mul` mean?

**Fog (not precisely answerable from current evidence):**

- The legal weight of Perseus's "overall copyright" statement alongside a CC BY-SA 4.0 license.
- Whether TLG-licensed text can ever be used in a self-hosted app under any terms (TLG access terms not retrieved).
- Whether the Morpheus lexicon behind AGDT and Diorisis carries its own license.
- Whether Diorisis's "automatic" annotation quality (candidate-level accuracy) is acceptable for frequency work; no accuracy evaluation was found in the repository.

## Publication blockers

None for the evidence file. Publication of the Diorisis-related conclusions depends on the unresolved license conflict above, which this file records rather than resolves.
