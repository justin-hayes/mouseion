# Third-party model, lexicon, treebank and text licence provenance

**Research date:** 2026-10-25

**Issue:** [Research: licence provenance of every third-party model, lexicon, treebank and text Mouseion uses](https://github.com/justin-hayes/mouseion/issues/2307). It feeds [Decide Mouseion's licence posture for third-party data across all languages](https://github.com/justin-hayes/mouseion/issues/2308).

**Scope:** every third-party model, embedding, character model, treebank, lexicon, dictionary, text and font that Mouseion uses today, or that its open plans propose. Facts only. No posture is recommended here.

**Builds on, not repeated:** [`doc/research/modern-greek-nlp.md`](../research/modern-greek-nlp.md); the French provenance note at [`c90b50de`](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md) (cited below as **FR-note**, with its own source IDs); the Ancient Greek lemmatisation note on [`research/grc-non-nc-lemmatisation`](https://github.com/justin-hayes/mouseion/blob/research/grc-non-nc-lemmatisation/doc/research/grc-non-nc-lemmatisation.md) (cited as **GRC-note**); and the graded-reader inventory on [`research/graded-greek-readers-inventory`](https://github.com/justin-hayes/mouseion/blob/research/graded-greek-readers-inventory/doc/research/graded-greek-readers-inventory.md) (cited as **Reader-inventory**).

**Ancient Greek source licence:** the OGA `oga_src` v0.2.0 licence and AGDT lineage are not re-verified here. See [Verify the AGDT version and licence behind the pinned OGA oga_src v0.2.0 annotations](https://github.com/justin-hayes/mouseion/issues/2306).

## Method and pins

Every licence below was read from a file or page at a stated revision, or was fetched on 2026-10-25 and recorded by SHA-256. Downloads are in a scratch directory outside the repo and were treated as data only. Where a page has no revision, that is stated.

| Source | Pin / access | Evidence |
|---|---|---|
| Stanza version | `stanza==1.15.0` in `nlp/uv.lock` | Repo |
| Stanza resource manifest | `stanfordnlp/stanza-resources@569db9d`, `resources_1.15.0.json`, sha256 `ae9741f2…4463` (matches FR-note) | Read `de`, `it`, `el`, `grc`, `fr` package and dependency tables |
| Stanza code, `default_packages.py`, `download_vectors.sh`, `LICENSE` | `stanfordnlp/stanza@v1.15.0` (commit `c9a2158`, per FR-note) | Fetched; `LICENSE` is the 603-byte header only, sha256 `9e65adb4…8a50` |
| Stanza HF model repos | `stanfordnlp/stanza-{de,it,el,grc,fr}` at tag `v1.15.0`: de `a8ecd6c8`, it `56584df8`, el `f7aa6d04`, grc `70a5a752`, fr `caeb7742` (FR-note) | HF API `cardData.license` = `apache-2.0` for all five |
| Stanza combined-models page | `stanfordnlp.github.io/stanza/combined_models.html`, no commit pin, fetched 2026-10-25, sha256 `3c38c4df…`| Lists treebanks per combined language |
| Stanza performance page | `stanfordnlp.github.io/stanza/performance.html`, no commit pin, fetched 2026-10-25, sha256 `2fdbbd3b…` | Licence statement quoted in §2 |
| conll17 word embeddings | LINDAT handle `11234/1-1989` (record fetched 2026-10-25) | Licence statement quoted |
| UD treebanks | `UniversalDependencies/UD_*` at `HEAD` on 2026-10-25 (German-GSD `ce54dbe9`, German-HDT `8359ac9b`, Italian-ISDT `ff2447f6`, Italian-VIT `12fc5f68`, Italian-PoSTWITA `b8237631`, Italian-TWITTIRO `ff4c6073`, Greek-GDT `005bb3e0`, Ancient_Greek-Perseus `331ddef9`, Ancient_Greek-PROIEL `a4ab8d43`, Ancient_Greek-PTNK `818fb315`) | Each `LICENSE.txt` and README metadata block |
| GreekBERT | HF `nlpaueb/bert-base-greek-uncased-v1` at `ec2b8f88` (no licence field); GitHub `nlpaueb/greek-bert` `master` at `2f0d84b6` (read 2026-10-25); `LICENSE.md` sha256 `722190d4…2895` | Read |
| Perseus canonical-greekLit | `PerseusDL/canonical-greekLit@0189b9c8` (README) | README |
| Perseus lexica (LSJ) | `PerseusDL/lexica@56061ca1` (README) | README |
| Morpheus | `perseids-tools/morpheus@ab6898ff` (LICENSE, MPL-2.0); `PerseusDL/morpheus@b1b33c56` (README, CC BY-SA 3.0 US) | GitHub |
| Repo | `origin/main` at `f63b3d3b` | `README.md`, `compose.yaml`, `Dockerfile`, `nlp/Dockerfile`, `nlp/src/mouseion_nlp/model_config.py`, `provision.py`, `internal/dictionary/attribution.go`, `internal/cardexport/cardexport.go`, `doc/operations.md`, `doc/features/language-support.md`, ADRs 0064, 0073, 0099, 0102 |

Note on GreekBERT's GitHub pin: the `master` branch was read on 2026-10-25 at `2f0d84b6`; the file's SHA-256 is recorded. That is a branch read, not a tagged release. That is a gap, listed in §4.

## 1. Where each artifact reaches the learner

**Repo and image.** `nlp/Dockerfile` installs locked Python packages (Stanza included) from `nlp/uv.lock` and copies `nlp/`, `proto/` and `gen/python/`. It copies no model file. The root `Dockerfile` copies `cmd/`, `internal/`, `proto/`, `gen/` and `migrations/`, so the embedded static assets under `internal/webapp/static/vendor/` ship in the web binary. `README.md` says the optional dictionary index "is not distributed with the source".

**Deploy time (provisioning).** `compose.yaml` runs `nlp-init` as `python -m mouseion_nlp.provision`. It downloads the Stanza packages for the warmed languages into the `stanza-data` volume. It also verifies GreekBERT in the `huggingface-data` volume, and it verifies French artifacts by sha256 (`provision.py`, `model_config.py`). Default `MOUSEION_NLP_WARM_LANGUAGES` is `de,it,el,fr`.

**Operator-built artifact, mounted read-only.** `make dictionary-index` derives the Kaikki/Wiktextract SQLite index (`doc/operations.md` "Dictionary index"). The operator fetches the raw dump (`--download`) or supplies `KAIKKI_INPUT`. The LSJ entries come from an operator-supplied `DICTIONARY_LSJ_DIR`. The result is mounted read-only into the web container (`compose.yaml` line ~108).

**Operator-supplied file, imported in the app.** The Ancient Greek importer (`internal/ancientgreek/import.go`) and the Brownson import (`internal/ancientgreek/brownson.go`) read a file the operator names. Each is checked against a pinned size and SHA-256. The learner then triggers the import, and the derived analyses are stored in PostgreSQL.

**Runtime download.** None found. The NLP service serves offline (`provision.py` and `language-support.md`, French section). The Kaikki download and `urlopen` appear only in the operator script `nlp/scripts/derive_dictionary_index.py`.

**Exported Anki (APKG).** `internal/cardexport/cardexport.go` writes notices into the deck description, not into each card (`deckDescription`, `attributionNotices`). The notices are:
- `dictionary.AttributionNotice` (Wiktionary/Kaikki) when a row has a dictionary provider version and candidate senses (`cardexport.go` ~1528, `dictionaryAttribution`);
- `dictionary.AncientGreekLSJNotice(pin)` when a matched LSJ entry is frozen (~1531);
- `dictionary.AncientGreekSourceNotice` when the deck is Ancient Greek (~1534);
- `dictionary.BrownsonSourceNotice` when a Brownson passage is frozen (~1537).

No notice names Stanza, the UD treebanks, conll17, GreekBERT or the `newswiki` character models. The `apkg.go` comment says the package contains no media, so fonts and models are not exported. Card fields include `Lemma` and `POS` (`cardexport.go` ~69). Their values come from the analysis, and that analysis can be Stanza output. This is an inference from the code path, not a direct test.

## 2. Summary table

Restriction kinds: **none**, **attribution**, **share-alike** (SA), **NC** (non-commercial), **research/academic only**, **unclear**, **conflicting**. A row is marked **FAIL (strict no-NC)** when a declared NC term reaches the artifact or a dependency it needs. **UNVERIFIED** means the licence or lineage could not be established from primary sources, so the row cannot pass a strict check.

| # | Artifact | Used by | Licence or terms (primary source, pin) | Restriction | How it reaches the learner | In APKG? | Strict no-NC |
|---|---|---|---|---|---|---|---|
| 1 | Stanza 1.15.0 code | `de`, `it`, `el`, `fr` | Apache-2.0 (Stanza `LICENSE`, `v1.15.0`; header only, full text per FR-note) | attribution | Image (`uv sync`) | No | Pass |
| 2 | Stanza `de` package (combined) — `stanza-de@v1.15.0` | Today, warmed | Model card `apache-2.0`. Performance page: "License information for models built from the UD data is unclear" and packs are "made available under the Open Data Commons Attribution License v1.0" *to the extent* Stanford has rights (performance page, unpinned). Training data: UD German-GSD (CC BY-SA 4.0), German Wiktionary-composed lemma data (combined-models page), conll17 pretrain and `newswiki` charlm (below) | conflicting | Deploy time (`nlp-init`) | No (model output in `Lemma`/`POS` fields) | **FAIL** (via conll17 in `pos` and `depparse`); the `newswiki` charlm is UNVERIFIED |
| 3 | Stanza `it` package (combined) — `stanza-it@v1.15.0` | Today, warmed | Model card `apache-2.0`; performance page as row 2. Training data: UD Italian-ISDT ("released … under the Creative Commons Attribution-NonCommercial-ShareAlike 3.0 Unported licence", `LICENSE.txt`); Italian-VIT (CC BY-NC-SA 3.0); Italian-PoSTWITA (CC BY-NC-SA 4.0, `LICENSE.txt`); Italian-TWITTIRO (CC BY-SA 4.0). `pos`, `depparse` and both charlms use conll17 | NC, conflicting | Deploy time | No | **FAIL** (NC treebanks ISDT, VIT, PoSTWITA; NC conll17) |
| 4 | Stanza `el` package (`gdt`) — `stanza-el@v1.15.0` | Today, warmed | Model card `apache-2.0`; performance page as row 2. Training data: UD Greek-GDT, "distributed under a CreativeCommons Attribution-NonCommercial-ShareAlike 3.0 license" (`LICENSE.txt`, "User agreement"). `pos` and `depparse` use conll17; `depparse` also needs GreekBERT (row 9) | NC, conflicting | Deploy time | No | **FAIL** (NC treebank and NC conll17) |
| 5 | Stanza `fr` package (combined) — `stanza-fr@caeb7742` | Today, warmed | Per FR-note §2 (not re-derived): model card `apache-2.0`; performance page "unclear" and ODC-By; treebanks GSD, ParisStories, Rhapsodie (CC BY-SA 4.0), Sequoia (LGPL-LR); conll17 (NC); `newswiki` (unidentified). Status: BLOCKED in FR-note | conflicting, NC | Deploy time, eight artifacts sha256-checked | No | **FAIL** (conll17 NC); `newswiki` UNVERIFIED |
| 6 | conll17 word embeddings (Stanza `pretrain/conll17.pt`, and the conll17 charlms of `it`) | `de`, `it`, `el`, `fr`, plus grc (not warmed) | LINDAT `11234/1-1989`: "the CC BY-SA-NC 4.0 license applies to the automatically generated annotations and word embeddings, not to the underlying data, which may have different license and impose additional restrictions." The record's file is "CC BY-NC-SA 4.0" (`performance`/LINDAT pages, fetched 2026-10-25) | **NC** | Deploy time, as Stanza `.pt` files in `stanza-data`; the tar is not fetched by Mouseion | No | **FAIL** |
| 7 | Stanza `newswiki` character models (`de` pos/depparse; `fr`) | `de`, `fr` | No licence found. Corpus unidentified. Full-text search at `c9a2158` found only `default_packages.py` (FR-note §2.4) | unclear | Deploy time | No | **UNVERIFIED** |
| 8 | Stanza `grc` packages (comparison; not warmed) | None (not in `model_config.py`, not in warm default) | `stanza-grc@v1.15.0` `apache-2.0`. `perseus` and `proiel` trained on NC UD treebanks; `ptnk` on CC BY-SA 4.0; all use conll17 (GRC-note) | NC | Not shipped | No | **FAIL** |
| 9 | GreekBERT `nlpaueb/bert-base-greek-uncased-v1` (external, used by `el` `depparse`) | `el` | HF card has no `license` field (`ec2b8f88`). Upstream repo `LICENSE.md` is MIT, "Copyright (c) 2020 NLP AUEB Group" (`master`, not commit-pinned; sha256 `722190d4…2895`) | none (MIT) | Deploy time, via `ensure_external_dependency` into `huggingface-data` | No | Pass for the licence, **UNVERIFIED** for training corpora |
| 10 | Kaikki / Wiktextract dictionary index (de, it, el, fr, grc entries) | All five dictionary-backed languages | Wiktionary text: CC BY-SA 4.0 or GFDL, Wikimedia Terms of Use §7 (FR-note S26). Wiktextract code: MIT (FR-note S29, `05f0d933`). Kaikki raw dump: no resource-specific statement found (FR-note S30). Dump 2026-09-02, extraction 2026-10-03 (ADR 0102) | SA (plus GFDL option) | Operator-built SQLite, mounted read-only | **Yes**: `AttributionNotice` in deck description | Pass (no NC term found) |
| 11 | LSJ (Perseus lexica, Greek cards) | `grc` cards | `PerseusDL/lexica@56061ca1` README: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License" | SA | Operator-supplied directory, compiled into the index | **Yes**: `AncientGreekLSJNotice` | Pass (as stated) |
| 12 | OGA `oga_src` v0.2.0 (Zenodo 15199546) | `grc` import | Zenodo record `cc-by-sa-4.0`; GitHub repo has no LICENSE file. AGDT lineage unverified (GRC-note) | SA | Operator-supplied CoNLL-U file, size and SHA-256 pinned, imported in-app | **Yes**: `AncientGreekSourceNotice` | **Not assessed here**; see [#2306](https://github.com/justin-hayes/mouseion/issues/2306) |
| 13 | Perseus canonical-greekLit, Brownson *Anabasis* `perseus-eng2` (`0189b9c8`, SHA-256 `f4c88203…6ea7`) | `grc` passage context | README: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License" (Perseus). The Brownson translation (Loeb, 1921–22) has no licence check here | SA; translation copyright unverified | Operator-supplied file, digest-pinned, imported in-app | **Yes**: `BrownsonSourceNotice` | Pass as stated; translation UNVERIFIED |
| 14 | UD German-GSD (inside row 2) | `de` | `LICENSE.txt` and README: "CC BY-SA 4.0"; "Google gave permission to drop the NC restriction … This applies to the UD annotations (not the underlying content …)". README: "Portions of the German data were sampled from the CoNLL 2006 Tiger Treebank" | SA | Via row 2 | No | Pass for GSD annotations; Tiger-sourced sentences UNVERIFIED |
| 15 | Morpheus (proposed; no runtime import in repo) | Not used | Fork `perseids-tools/morpheus@ab6898ff`: MPL-2.0. Upstream `PerseusDL/morpheus@b1b33c56`: "CC BY-SA 3.0 United States". Stemlib data lineage and data licence not verified (GRC-note) | MPL file-level; SA | Would be deploy-time | — | Not NC; data UNVERIFIED |
| 16 | Graded readers: six GLTP repos (proposed) | Not used | CC BY-SA 4.0 (LICENSE files; Reader-inventory §3.1). Print PD status UNVERIFIED for UK and EU | SA | Would be operator-supplied | — | Pass as stated |
| 17 | Graded readers: Exocortex digitisation (proposed) | Discovery only | Repo README: texts PD; "the digitisation layer … is licensed CC BY-NC 4.0" (Reader-inventory §3.2) | NC | Not used | — | **FAIL** |
| 18 | Graded readers: LGPSI and EasyGreekReaders (proposed) | Not used | No LICENSE file; GitHub `license: null`; no licence statement (Reader-inventory §3.2) | unlicensed | Not used | — | Excluded (no licence) |
| 19 | Literata, Commissioner (fonts) | Web UI | SIL OFL (licence files in `internal/webapp/static/vendor/fonts/`; the vendor README records the OFL) | OFL (attribution and reserved-name terms; text not re-read here) | Embedded in web binary | No (no media in APKG) | Pass |
| 20 | htmx 4.0.0 | Web UI | Zero-Clause BSD (`vendor/licenses/htmx-4.0.0-LICENSE.txt`) | none | Embedded in binary | No | Pass |
| 21 | Tailwind CSS, daisyUI | Web UI (compiled `app.css`) | MIT (`vendor/licenses/`) | attribution | Compiled CSS shipped in repo | No | Pass |
| 22 | `internal/canonicalization/german_post1996.json` | `de` canonicalisation; copied into `nlp` image | In-repo data; provenance not assessed | unclear | Image | No | UNVERIFIED |

Rows 13, 17 and 18 are not warmed or shipped by default; row 17 is not used at all. Rows 8 and 15–18 are not used by the shipped configuration today.

## 3. Artifacts that fail a strict no-NC rule

- **Stanza `de`** (row 2): conll17 embeddings in `pos` and `depparse`.
- **Stanza `it`** (row 3): ISDT, VIT (CC BY-NC-SA 3.0), PoSTWITA (CC BY-NC-SA 4.0) and conll17.
- **Stanza `el`** (row 4): GDT (CC BY-NC-SA 3.0) and conll17.
- **Stanza `fr`** (row 5): conll17 (NC), per FR-note.
- **conll17 embeddings** (row 6): NC by the LINDAT statement.
- **Stanza `grc` packages** (row 8): Perseus and PROIEL treebanks and conll17. Not used.
- **Exocortex digitisation** (row 17): NC. Not used.

The NC term is not confined to the treebanks. It reaches every warmed Stanza package through conll17 (rows 2–5). This is recorded from the LINDAT statement; whether it reaches fine-tuned weights is the open question in FR-note §2.2.

## 4. Unverifiable items, stated plainly

- **`newswiki` character models** (`de`, `fr`): no licence found and the training corpus is unidentified.
- **conll17 underlying text:** the LINDAT record says the NC licence "applies to … word embeddings, not to the underlying data, which may have different license". The underlying raw text is not identified in the record.
- **Stanza licence conflict:** the model cards say Apache-2.0; the performance page says the UD-derived licence is "unclear" and points to ODC-By. The performance and combined-models pages are unpinned live pages (fetched 2026-10-25), not versioned.
- **UD German-GSD Tiger sentences:** the source licence of the Tiger-sampled sentences was not checked.
- **Stanza German lemma data from Wiktionary:** the combined-models page says it is "composed from German Wiktionary". Its licence rests on Wikimedia terms (FR-note S26); no Stanza-side licence statement was found.
- **GreekBERT:** the HF card has no licence field; MIT comes from a `master` file, not a commit pin. Training corpora not verified.
- **Kaikki raw dump:** no resource-specific licence statement found (FR-note S30).
- **Brownson translation** (Loeb, 1921–22): copyright status not verified.
- **OGA AGDT lineage and version:** deferred to [#2306](https://github.com/justin-hayes/mouseion/issues/2306).
- **Morpheus data and stemlib lineage:** not verified (GRC-note).
- **PROIEL licence-text conflict:** `UD_Ancient_Greek-PROIEL/LICENSE.txt` says "Attribution-NonCommercial-ShareAlike 3.0 Generic" while its URL points to the 4.0 legal code, and its README says "CC BY-NC-SA 3.0". GRC-note records 4.0. Not resolved.
- **Print PD status** for graded readers and the Brownson translation: not verified in the UK or EU.
- **LLM glosses and translations** (provider terms) are outside this scope and not assessed.
- **Python package licences** (other than Stanza) and the `torch` wheel are outside this scope.
- **Actual file contents:** no model file was downloaded or opened. Every claim is from licence text, READMEs, manifests and code.

## 5. Downstream notes

- The repo's own licence is AGPL-3.0-only (`README.md` "License", `nlp/pyproject.toml`). This is not third-party data.
- ADR 0102 and the French section of `doc/features/language-support.md` both state that no French licence question is resolved and that redistribution is unsupported. This note does not change that.
- The `Lemma`/`POS` fields in exported decks carry model output with no Stanza, UD or conll17 notice. The deck-level notices name Wiktionary, LSJ, OGA and Brownson only.
