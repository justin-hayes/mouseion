# French Resource Provenance and Permitted Uses

**Research date:** 2026-10-17

**Tickets:** #2153 (this research), #2138 (map), #2141 (adoption decision gate).

**Scope:** Exact provenance, license evidence, and obligations for the two French
resources in the provisional checkpoint agreed in #2141:

1. The Stanza 1.15.0 French default package (`fr`, `combined` processors).
2. Raw English Wiktionary as extracted by Wiktextract and published by Kaikki.org.

Named-entity recognition (`ner`) and coreference (`coref`) are out of scope.

**Status: BLOCKED for adoption.** This note records evidence and open questions.
It does not grant permission to use, download, bundle, derive, or distribute
either resource. Final adoption needs a human decision in #2141 that accepts a
documented basis.

**Not done:** no model bundle or Kaikki dump was downloaded. No checksum of a
downloaded file was computed. Only HTTP HEAD requests, small metadata files, and
primary-source pages were read. Nothing in this note is legal advice.

## Evidence labels

| Label | Meaning |
|---|---|
| **[V]** | Read or fetched from the primary source during this pass (2026-10-17). |
| **[V-prior]** | Read in an earlier pass on this ticket (scratch notes); not re-fetched here. |
| **[M]** | Metadata reported by a registry or API (manifest, Hugging Face tree, HTTP headers). Not verified against the file bytes. |
| **[I]** | Inference: a mapping or reading the sources do not state directly. |
| **[L]** | Legal interpretation. Needs human or counsel review. |
| **[G]** | Gap: evidence not found, not checked, or not resolvable from public sources. |

## Bottom line

- **Code and model metadata are not the whole picture.** The Stanza code is
  Apache-2.0 [V]. The Hugging Face model card says `apache-2.0` with no
  training-data notice [V-prior]. Stanza's own performance page says the
  UD-derived model license is "unclear" and that, "to the extent" Stanford has
  rights, the packs are ODC-By 1.0 [V-prior]. These statements conflict or are
  unresolved. This note does not settle which one governs the weights.
- **Two copyleft-style inputs need attention.** The French training treebanks
  GSD, ParisStories, and Rhapsodie are CC BY-SA 4.0 [V-prior]. The conll17
  pretrained vectors used by the French `pos` and `depparse` models trace to a
  LINDAT item licensed CC BY-NC-SA 4.0 [V-prior]. Whether either license
  reaches the Stanza weights is **not determined** here [L][G].
- **Wiktionary text is CC BY-SA 4.0 and GFDL, with per-entry exclusions.**
  Wiktionary's own page describing this is marked outdated and as an unofficial
  draft [V]. Quotations and other third-party material keep their own terms
  [V]. Wiktionary's quotation policy was not read [G].
- **Kaikki has a citation request, not a found license declaration.** Searched
  the French index, raw-data page, and home page: the only use terms found are
  an academic-citation request [V]. That is evidence about these pages, not
  proof that no resource-specific license exists elsewhere [G].
- **Private single-user use is the least constrained case.** CC BY-SA 4.0
  attaches its Attribution and ShareAlike conditions to "Sharing," which it
  defines as making material available "to the public" [V]. Single-user private
  use, and transfer to one's own device, may fall outside that definition [L].
  Public distribution, public APKG sharing, and any hosted or distributed
  artifact are different. So is any LLM-provider processing whose terms are
  unreviewed [G].

## 1. Exact dependency artifacts

### 1.1 Pinned manifest

| Field | Value | Label |
|---|---|---|
| Source | `stanfordnlp/stanza-resources`, file `resources_1.15.0.json` at commit `569db9d983a47931673d7d265361ca93af85d176` (main, committed 2026-10-01T02:51:50Z) | [V] (commit and date from the GitHub API) |
| Manifest sha256 | `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463` | [V] (computed on the fetched copy) |
| Manifest md5 | `462ea52f652d309152a102ca15ffd942` | [V] |
| `fr` default md5 | `1d68653a7e1bbe220b3f1e1d0a2a07c6` | [V] (in the manifest) |
| Tag check | The `stanza-resources` tags endpoint returned 404 (no tag found). Not checked: the manifest the 1.15.0 wheel uses. | [G] |
| History | The `fr` section changed in commit `e8941fa8add` (2026-09-29): all French md5s regenerated and the default tokenizer changed from `combined` to `combined_nocharlm`. The `fr` section is identical from that commit to `569db9d`. | [V] (earlier pass) |
| Fetch behavior | Stanza's default resources URL is `main`, so the manifest is a moving target unless a commit is pinned. | [V-prior] |

**Requirement:** the pin must record the stanza-resources commit SHA and the
manifest's sha256 when a model is provisioned, not the name `main`.

### 1.2 Default `fr` package → artifacts

The mapping below is read from the manifest. The **HF sha256** column is the LFS
oid from the Hugging Face tree for `stanfordnlp/stanza-fr` at revision `v1.15.0`
[M]. The **md5** column is the manifest value [M]. Neither was computed from
bytes here.

| Role | Processor | Artifact (path in `stanza-fr` @ v1.15.0) | Manifest md5 [M] | HF LFS sha256 [M] | Size (bytes) [M] | Dependencies |
|---|---|---|---|---|---|---|
| tokenize | `combined_nocharlm` | `models/tokenize/combined_nocharlm.pt` | `34ce2e674142753bb26d2bd66adc7742` | `622a7613d78623cda63e53fc72f84c49b2f64ad959c0f6aeb51355b963eda78f` | 665712 | none |
| mwt | `combined` | `models/mwt/combined.pt` | `827fd37941d9bb51bdd7c473e0e80cf8` | `a46bc4149531762c2fd79d70eeaaff80479aa6852a08516ab0db567859c8cec0` | 544773 | none |
| lemma | `combined_nocharlm` | `models/lemma/combined_nocharlm.pt` | `07f18945c29490c60ff3a9dfa18fbdc5` | `31dd152c13d1d713085127e7b30d91e70e88652da4c2ea33c4b6f7d34d810177` | 2813969 | none |
| pos | `combined_charlm` | `models/pos/combined_charlm.pt` | `505cd655d0474c51d1e35312d49bceae` | `e40eb06786fa691e84aa6ee1ff2321b37f9ff9cb9bf3fe4de94b63793b626f89` | 34563706 | `pretrain/conll17`, `forward_charlm/newswiki`, `backward_charlm/newswiki` |
| depparse | `combined_charlm` | `models/depparse/combined_charlm.pt` | `c0c8d747238802ee03c40953a8e4d101` | `5ad85cd10e385894152154863c14f621b7d747c67c84eb8597039da12832315a` | 148809357 | same as pos |
| pretrain | `conll17` | `models/pretrain/conll17.pt` | `f74659129e2008c6b7b6ca0a2db693d7` | `c8c456c1a48fba32d4ab827ae7d3fec40457f58ead7e1977b6267afc37474a3c` | 106908071 | none |
| charlm forward | `newswiki` | `models/forward_charlm/newswiki.pt` | `d98e6909619100c23b745cae4383eea2` | `a6874f7670f84c8bd9e57d5dcbafdfe7d7cbae2457c79975040709f1d69ca119` | 20094072 | none |
| charlm backward | `newswiki` | `models/backward_charlm/newswiki.pt` | `42eba035046673e195274f45e713653c` | `3ae24cade5cdca7d68193196a942a177af81fbc5d3b5ebb4550ff7b89023b09c` | 20094073 | none |

Notes on this mapping:

- **[V]** The manifest's default `fr` entry names these processors, and the
  dependencies listed above are read directly from the manifest.
- **[I]** The default French pipeline uses `combined_charlm` only for `pos` and
  `depparse`. The `newswiki` character language models are therefore needed
  because of those two processors, not because of tokenize, mwt, or lemma.
  This follows from the manifest entries.
- **[I]** The `conll17` pretrained vectors are a dependency of `pos` and
  `depparse` in all their variants, including `nocharlm`. Dropping charlm does
  not remove the conll17 dependency.
- **[V-prior]** The `combined` training set is GSD, ParisStories, Rhapsodie,
  and Sequoia. ParTUT is not in the default.
- **[G]** The sha256 values above are from the Hugging Face tree API, not from
  downloaded bytes. Recomputing them would require downloading about 300 MB. That
  was deliberately not done in this pass.
- **[G]** The mapping of `pretrain/conll17.pt` to the LINDAT vectors is not
  verified. Stanza's documentation and the file name support the link [I], but
  no byte-level comparison was made.
- **[G]** The manifest's `ner` entry also depends on `conll17`. NER is out of
  scope.

## 2. Model and code license evidence

### 2.1 Notices found

| Resource | What the source says | Label |
|---|---|---|
| Stanza code (`stanfordnlp/stanza`) | Apache-2.0 | [V] (repo LICENSE) |
| Hugging Face `stanfordnlp/stanza-fr` model card | `license: apache-2.0`; no training-data notice (checked at tag `v1.15.0` and `main`) | [V-prior] |
| Stanza performance page | UD-derived model license "unclear." To the extent Stanford has rights, the packs are under ODC-By 1.0. | [V-prior] |
| GSD treebank | CC BY-SA 4.0 (`LICENSE.txt` and README). README says the underlying text was collected by Google "as is" and may be copyrighted. | [V-prior] |
| ParisStories, Rhapsodie treebanks | CC BY-SA 4.0 | [V-prior] |
| Sequoia treebank | LGPL-LR. Its sources, per the README, include Europarl, French Wikipedia, L'Est Républicain, and EMEA. | [V-prior] |
| conll17 French vectors (LINDAT item `053071c7-84f7-4d0b-bdd7-f87cbafcd3a9`) | CC BY-NC-SA 4.0. The item says the license covers generated annotations and embeddings but not the underlying data. | [V-prior] |
| NLPL repository | Lists French CoNLL17 as ID 43 | [V-prior] |
| `newswiki` character language models | Training corpus **not identified** in Stanza 1.15.0 sources | [G] |

### 2.2 Conflicts and unresolved points

1. **Apache-2.0 (code and HF card) vs. "unclear" / ODC-By "to the extent"
   (Stanza performance page).** These do not agree on the weights. The HF card
   has no training-data notice. Neither source says which one governs the
   packaged `.pt` files [G][L].
2. **Share-alike from CC BY-SA treebanks.** Whether trained weights count as
   "Adapted Material" under CC BY-SA 4.0 §1(a) is not settled [L]. The license
   text ties ShareAlike to Sharing Adapted Material [V].
3. **Non-commercial inheritance from the conll17 vectors.** [I] The `pos` and
   `depparse` models use these vectors, so if the vectors' CC BY-NC-SA terms
   reach the fine-tuned weights, they would restrict commercial use. [L] Whether
   they reach the weights, or whether the LINDAT item's own "not the underlying
   data" carve-out limits that, is not settled. Stanza's link from the `.pt` file
   to the LINDAT item was not byte-verified [G].
4. **Sequoia LGPL-LR.** Its applicability to weights trained on Sequoia data is
   not analyzed here [L].
5. **Underlying text rights.** The treebanks' text, not just their annotations,
   may carry third-party rights (GSD README) [V-prior] [L].

### 2.3 Use-level consequence for models

Use-by-use obligations are in §4. In short: no model license conclusion is
settled for any use beyond private execution.

## 3. Wiktionary and Kaikki evidence

### 3.1 Wiktionary

- **[V]** `Wiktionary:Copyrights` says original entry text is dual-licensed under
  CC BY-SA 4.0 and GFDL, and that material from external sources keeps its own
  terms and must be identified as external.
- **[V]** The same page carries a notice that it is outdated, dates to 2002, and
  is an unofficial draft. This note therefore cites the license text, not the
  page, as the operative term.
- **[V]** The en.wiktionary footer says definitions and other text are under
  CC BY-SA, and that "additional terms may apply." Those additional terms are
  not enumerated on the footer.
- **[V]** The same page describes quotations of contemporary authors as fair use
  that must still be identified by source. Quotations are therefore separate
  third-party material, not CC BY-SA text by default [L].
- **[G]** Wiktionary's quotation policy (`WT:QUOTE`) was **not read**. Per-entry
  quotation handling is unverified.
- **[G]** Imported external text and history-page attribution were not sampled
  on individual entries.
- **[V-prior]** Wiktextract's code is MIT-licensed. Its `tests/` files are under
  Wiktionary's license.

### 3.2 Kaikki.org

- **[V]** Searched the French dictionary index, the raw-data page, and the home
  page for "license," "copyright," "CC," "attribution," and "citation." Found
  only an academic-citation request for Ylonen (LREC 2022), on each page. That is
  a request, not a license condition [L].
- **[G]** No resource-specific license declaration was found on these pages. The
  search does not show that none exists anywhere; the rest of kaikki.org and the
  Wiktextract repository were not exhaustively read for one.
- **[V-prior]** The post-processed data includes "additional data merged from
  other sources," so it is not purely Wiktionary-derived [V-prior]. The raw
  English dump was the input of the repo script [V].
- **[V]** HEAD on the raw English dump
  `https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz`: HTTP 200,
  `content-length: 2981058381`, `last-modified: Sat, 03 Oct 2026 08:24:38 GMT`,
  `etag: "6ac0bbc6-b1af574d"`. These are headers only, not a content hash [M].
- **[V]** The raw-data page lists the dump dated 2026-09-02 and extracted
  2026-10-03. A raw French-edition extract is listed as work in progress.
- **[G]** No sha256 of the raw dump was obtained. The `etag` is not a content
  hash.

### 3.3 Repository facts (checked in this pass)

- `nlp/scripts/derive_dictionary_index.py:21` `LANGUAGES = {"de", "it", "el"}`.
  French is not a derivation target [V].
- `nlp/scripts/derive_dictionary_index.py:29` `KAIKKI_DOWNLOAD_URL` is the raw
  English dump (~2.8 GB compressed) [V].
- `nlp/scripts/derive_dictionary_index.py:521` and `:524` hard-code the
  metadata strings `license: "Wiktionary-derived data: CC BY-SA 4.0 / GFDL"` and
  `attribution: "Wiktionary contributors; CC BY-SA 4.0 / GFDL"` [V]. These do not
  record a snapshot.
- `nlp/scripts/derive_dictionary_index.py:615` writes a version string of the
  form `dump=…;extraction=…;wiktextract=…`. It can record the dump date, the
  extraction date, and the wiktextract commit when supplied [V].
- `nlp/scripts/derive_dictionary_index.py` `sense_from` keeps the first gloss,
  the verbatim examples, IPA, plural, gender, and article [V-prior].
- `internal/dictionary/attribution.go:4` `AttributionNotice` names Wiktionary,
  Wiktextract, and Kaikki.org, and says CC BY-SA 4.0 / GFDL with share-alike. It
  has **no** dump date, extraction date, or wiktextract commit [V].
- `internal/cardexport/cardexport.go:1609–1634` (`deckDescription`,
  `attributionNotices`) adds `AttributionNotice` to APKG descriptions and TSV
  headers when dictionary evidence is present [V].
- `internal/cardexport/cardexport.go:1531` (`selectedSenseGloss`) can place a
  verbatim, token-truncated Wiktionary sense gloss into card Gloss via
  `enrichment.RenderGloss` [V].
- `internal/cardexport/cardexport.go:1468` (`applyContextualGloss`) is the LLM
  path, used when `MeaningEvidenceHash` is set [V]. Which path a given deck
  uses is for maintainers to confirm [G].
- `internal/enrichment/llm_codec.go:180–196` sends `CandidateSenses` (glosses and
  examples) in the LLM request payload [V].
- `doc/adr/0079-contextual-glosses-require-llm.md:55` says Wiktionary attribution
  "stays in deck/export metadata, not on each card." Lines 72–74 say the existing
  index "retains its attribution and redistribution terms" [V].
- `README.md` §License (lines ~246–259) says the optional dictionary index is not
  distributed with the source and is CC BY-SA 4.0 / GFDL when built [V].

## 4. Use-by-use analysis

The five uses below follow the original ticket. For each, "obligations" lists
what the sources say must be done when that use occurs. "Gaps" lists what is
unresolved.

### Use 1: Personal self-hosted execution

- **Model (Stanza fr):** running the pipeline privately on one's own machine,
  without distribution. Obligations from the sources: none stated for private
  use in Apache-2.0, ODC-By, or CC licenses [V][L]. The CC BY-NC-SA "non-commercial"
  restriction is about commercial use [L]. Under CC BY-SA 4.0 §3 the attribution
  and ShareAlike conditions attach only to Sharing [V].
- **Dictionary (Wiktionary/Kaikki):** a private, local index. The same logic
  applies [L].
- **Gaps:** [G] whether any stated "unclear" or ODC-By term imposes use-level
  obligations; [G] whether the project's public release (AGPL-3.0 README) affects
  the analysis for a user who runs the software for others.

### Use 2: Automated provisioning and download

- **Current state:** `nlp/src/mouseion_nlp/provision.py` contains no md5,
  sha256, or checksum verification string, and no license-notice writing
  [V]. `provision.py` also contains no reference to `stanza-resources` [V].
  The provisioning path therefore has no pin and no integrity check in the code
  reviewed here [G][I].
- **Obligations:** pin the stanza-resources commit and manifest sha256; verify
  each artifact's md5 (and sha256 where available) before use; write a notice
  file per artifact recording the license evidence in §2 [I].
- **Gaps:** [G] the Stanza loader's own checksum behavior was not read; [G] any
  automated download of conll17 or the CC BY-NC-SA-linked artifact is not
  cleared by this note.

### Use 3: Local dictionary derivation and refresh

- **Current state:** the derivation script downloads the 2.8 GB raw dump,
  derives an index, and writes hard-coded license metadata [V]. `fr` is not in
  `LANGUAGES` [V].
- **Obligations (for private use):** record the dump date, extraction date,
  wiktextract commit, and dump sha256 in the index metadata for each refresh
  [I]. Keep the attribution with the derived index [I].
- **Gaps:** [G] the dump sha256 was not computed; [G] whether per-entry
  quotations and imported text are excluded; [G] the `etag` is not a content hash.

### Use 4: Distribution of models, data, or derived dictionaries

**Not a launch destination. This section records what would apply.**

- **Model weights:** possible obligations include Apache-2.0 notice (code, [V]),
  ODC-By notice (if it applies, [V-prior][L]), CC BY-SA ShareAlike for adapted
  material if the weights are adapted from GSD/ParisStories/Rhapsodie ([L]), and
  CC BY-NC-SA non-commercial restriction if the conll17 terms reach the weights
  ([L]). Sequoia LGPL-LR may apply ([L]).
- **Dictionary-derived data:** CC BY-SA 4.0 Sharing requirements: attribution
  (§3(a)(1)), indication of modification (§3(a)(1)(B)), license text or link
  (§3(a)(1)(C)), and ShareAlike for the adapted index (§3(b)) [V][L]. Kaikki's own
  terms, if any, are not found [G].
- **Gaps:** [G] whether the derived index is "Adapted Material" under CC BY-SA;
  [G] whether the Kaikki post-processing merge changes that; [G] whether the
  raw dump's per-entry quotations fall under CC BY-SA at all.

### Use 5: Dictionary-derived material in APKG exports

This use splits in two, and the two cases differ.

**5a. Copied fields.** Verbatim or near-verbatim Wiktionary text placed in
exported card fields (for example, `selectedSenseGloss` in Gloss; verbatim
examples; IPA; gender; plural) [V].

- **Single-user private transfer** (the user exports an APKG and imports it on
  their own device). CC BY-SA 4.0 §1(k) defines "Share" as making material
  available "to the public" [V]. Attribution and ShareAlike apply under §3 when
  one Shares [V]. Private transfer to oneself may fall outside Sharing [L]. This
  is a reading of the definition, not a settled conclusion.
- **Public sharing of the APKG** (for example, uploading it to a deck-sharing
  site, or giving it to others). This is Sharing [L]. Attribution in a reasonable
  manner is required (§3(a)(2)) [V]. The current per-deck description and TSV
  header notice may or may not be "reasonable to the medium" [L][G]. Share-alike
  for the APKG as adapted material is [L].
- **Quotations and external material.** Verbatim examples that are quotations
  from third-party works keep their own terms [V]. Their use in an export that is
  Shared is not cleared [G].

**5b. Generated or contextual fields.** LLM-generated glosses grounded in
candidate Wiktionary senses (ADR 0079, `applyContextualGloss`) [V].

- Whether LLM output is "Adapted Material" based on Wiktionary text is [L]. The
  answer depends on how much of the source text the output retains.
- **LLM-provider transmission.** Candidate senses (glosses and examples) are sent
  to the configured provider [V]. Whether that is Sharing under CC BY-SA is [L].
  The sharing definition is "to provide material to the public" [V], and provider
  processing for one user is not obviously "to the public," but the provider's
  terms, retention, and training use were **not reviewed** [G].
- **Attribution location.** ADR 0079 places Wiktionary attribution in deck/export
  metadata, not on each card [V]. Whether that satisfies §3(a)(2) for generated
  fields is [L].

## 5. Attribution path (feasible; not implemented)

1. **Per-artifact NOTICE for the Stanza bundle.** Provisioning writes a notice
   file with each artifact's processor, path, md5, and (when available) sha256;
   the stanza-resources commit; and the license evidence from §2 with source
   links. Owner: NLP provisioning. Requires §2 conclusions before it is
   authoritative.
2. **Versioned dictionary notice.** Extend `AttributionNotice` so that it includes
   the dump filename, dump date, extraction date, wiktextract commit, and dump
   sha256. The script already has the fields to write this [V]. Owner: dictionary
   and export.
3. **CC BY-SA link.** Include the license URI and a Wiktionary URL for each entry
   where practical. The entry URL can be constructed from the title; the raw data
   field for that was **not verified** [G].
4. **Exclusion list.** Drop or flag quotation examples, imported external text,
   and any sense that the per-entry Wiktionary history marks as external [G]. This
   needs a per-entry classifier that does not exist.
5. **LLM path.** Record in the export that contextual glosses were produced by a
   named provider from candidate senses, and link the notice [I].

## 6. Human and maintainer questions

These must be answered before #2141 can accept a basis.

1. **Stanza weights.** Does the project accept the Apache-2.0 code and HF card
   as the basis for the packaged French weights, given the ODC-By "to the extent"
   and "unclear" statements? Which Stanford statement is authoritative?
2. **conll17 vectors.** Does the project accept the risk that CC BY-NC-SA 4.0
   reaches `pos` and `depparse`? If not, which processor variant avoids conll17?
   (`combined_nocharlm` for `pos` and `depparse` still depends on conll17 per §1.2.)
3. **Treebank share-alike.** Does counsel conclude that CC BY-SA on GSD,
   ParisStories, and Rhapsodie does or does not reach trained weights?
4. **Sequoia.** Does LGPL-LR apply to weights trained on Sequoia data?
5. **Newswiki charlm.** What is the training corpus for the `newswiki` character
   language models? (Not identified in Stanza's 1.15.0 sources.)
6. **Manifest pin.** Which stanza-resources commit should the project pin, and
   should provisioning verify md5 or sha256 before loading?
7. **Kaikki.** Does the Kaikki maintainer state a license for the raw dump or
   the derived extracts? The citation request is not a license.
8. **Kaikki merge.** Does "additional data merged from other sources" in the
   post-processed data affect the raw English dump, which is the script's input?
9. **Quotations.** Does the project exclude quoted third-party examples from
   derived dictionary entries, and from exports?
10. **Private vs. public APKG.** Does the project treat a user's private APKG
    transfer as Sharing? Does it treat public deck sharing as Sharing?
11. **LLM provider.** Which provider is in use, and what are its terms for
    retention, logging, and training on submitted candidate senses?
12. **Attribution placement.** Is a per-deck notice "reasonable to the medium"
    under CC BY-SA 4.0 §3(a)(2) for generated fields, or is per-card attribution
    required?
13. **Scope.** Should French be held out of the dictionary index until 3 and 9 are
    answered?

## 7. Gaps summary

| # | Gap | Blocks |
|---|---|---|
| G1 | conll17 → LINDAT byte-level link not verified | Use 2, 4 |
| G2 | `newswiki` training corpus not identified | Use 2, 4 |
| G3 | Stanza 1.15.0 wheel manifest and tag not checked | Use 2 |
| G4 | No downloaded bytes; sha256 values are HF metadata only | Use 2 |
| G5 | Kaikki raw dump sha256 not computed | Use 3, 4 |
| G6 | Kaikki: no resource-specific license found on three pages; not exhaustive | Use 3, 4, 5 |
| G7 | Wiktionary quotation policy (`WT:QUOTE`) not read | Use 3, 4, 5 |
| G8 | Per-entry Wiktionary history and external-source marking not sampled | Use 3, 4, 5 |
| G9 | LLM provider terms not reviewed | Use 5b |
| G10 | Attribution in `provision.py` and loader checksum behavior not read | Use 2 |

## 8. Status

- **Stanza 1.15.0 French default package:** provenance mapped (§1); license
  terms conflicting or unresolved (§2). **Not cleared.**
- **Raw Wiktionary via Kaikki:** Wiktionary terms identified (CC BY-SA 4.0 /
  GFDL); Kaikki-specific terms not found (§3). **Not cleared.**
- **Adoption:** **BLOCKED.** Requires a human decision in #2141 that accepts a
  documented basis for each use the project intends.
