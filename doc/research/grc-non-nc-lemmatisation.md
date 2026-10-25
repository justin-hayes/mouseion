# Non-NC lemmatisation and morphology for unanalysed Ancient Greek

**Research date:** 2026-10-25

**Ticket:** [#2301](https://github.com/justin-hayes/mouseion/issues/2301) (parent map [#2299](https://github.com/justin-hayes/mouseion/issues/2299))

**Scope:** Which lemmatisation and morphology sources can produce Ancient Greek (`grc`) analyses for unanalysed reader text, under the map's no-NC rule (licence of the tool **and** of its full training-data or lexicon lineage). Context: [ADR 0099](../adr/0099-ancient-greek-imported-analyses.md) and [#2030](https://github.com/justin-hayes/mouseion/issues/2030).

## Recommendation

1. **Morpheus, pinned, as the only candidate that analyses arbitrary new text offline.** Pin `perseids-tools/morpheus` at commit `ab6898ffed335fc6169fa02c9940657a9b5a78e0`. Its code is MPL-2.0 and its repository licence file is MPL-2.0. Its upstream ancestor, `PerseusDL/morpheus` at `b1b33c56ef2338fe0dcd1893628ed638f00c0986`, declares CC BY-SA 3.0 US. Neither grant is NC. It returns every candidate parse for a word, so ambiguity is preserved and nothing is disambiguated. Input is beta code.
2. **Keep OGA `oga_src` v0.2.0 (already pinned under ADR 0099) for texts it already covers.** It is a pre-annotated corpus, not a lemmatiser, so it cannot analyse text outside its pinned works. Confirm the pin excludes AGDT 1.7-derived material (see below).

Neither path is a statistical disambiguator. No candidate with a verified non-NC lineage resolves parse ambiguity for new text.

## Verdict table

| Candidate | Tool licence | Data lineage | Verdict |
|---|---|---|---|
| **Morpheus** (perseids-tools fork) | MPL-2.0 (`LICENSE`, `a612ad9813b006ce81d1ee438dd784da99a54007`) | Stemlib data inside the same repo; origin lexica unverified (see below) | **Conditional pass (recommended)** |
| **Morpheus** (PerseusDL upstream) | CC BY-SA 3.0 US (`README.md`, `b1b33c56…`) | Same stemlib lineage | Pass as provenance statement, not as a separate pin |
| **OGA `oga_src` v0.2.0** (Zenodo 15199546) | CC BY-SA 4.0 per Zenodo record; GitHub `mgbilby/oga_src` has no detectable LICENSE file | AGDT-based morphosyntax (`oga-agdt-*`); Perseus/First1KGreek texts | **Pass (already accepted, ADR 0099)** |
| **Stanza `grc`** (all packages: `perseus`, `proiel`, `ptnk`, `default_*`) | Apache-2.0 (`stanza` code; HF `stanfordnlp/stanza-grc`) | `perseus`, `proiel` trained on UD treebanks that are NC; **every grc package uses the `conll17` pretrain**, which is NC (below) | **Fail** |
| **Stanza `grc_ptnk`** | Apache-2.0 | UD PTNK is CC BY-SA 4.0, but the pretrain is `conll17` (NC); PTNK is Septuagint Koine, not Attic | **Fail** (NC embeddings) |
| **OdyCy** `grc_odycy_joint_{sm,trf}` | MIT (HF `license:mit`, code and models) | Trained on UD Perseus and PROIEL (`scripts/clean/clean_treebanks.sh`); lemma lookup from spaCy lookups data (MIT, grc origin unverified) | **Fail** (NC treebanks) |
| **greCy** `grc_{perseus,proiel}_{sm,lg,trf}` | MIT | Trained on UD Perseus and PROIEL (its README says so) | **Fail** (NC treebanks) |
| **CLTK `grc_models_cltk`** (TnT tagger) | MIT | Trained on `greek_treebank_perseus`, which states **AGDT 1.7 is CC BY-NC-SA 2.5** | **Fail** (NC treebank) |
| **TreeTagger Ancient Greek parameter file** | Free for research/education/evaluation only; commercial by agreement (not an open licence) | "trained on the PROIEL and Perseus treebanks … no lemmas" | **Fail** |
| **Diorisis Ancient Greek Corpus** (figshare 6187256) | Corpus CC BY 4.0 (figshare) | Mixes Perseus canonical-greekLit (BY-SA) and other collections; its lemma layer was produced with TreeTagger (above), so its lemma lineage is unestablished | **Unresolved**; do not use its lemmas without a verified annotation provenance |
| **GLTP / jtauber Thesaurus Morphologicus Linguae Graecae** (`jtauber/Thesaurus_Morphologicus_Linguae_Graecae`) | **CC BY-NC-SA** (repo README) | Derived from LSJ | **Fail** |
| **Gorman `Greek-Dependency-Trees`** | Conflicting: README BY-NC-SA 4.0 vs `TREEBANK_LICENSE` CC0 (per #2128) | Hand-annotated treebank | **Unresolved** (not a lemmatiser; conflict stands) |
| **Dilemma** | Not located: no public repository found by `gh search` or web search | — | **Unverified**, no pass |
| **jtauber `postag-convert`** | MIT | Conversion library, no lexicon | Usable utility, not an analyser |
| **jtauber `greek-lemma-mappings`** | CC BY-SA 4.0 | NT headword mapping (MorphGNT / Nestle 1904) | Not an analyser; Koine |
| **CLTK `greek_lexica_perseus`** | MPL 1.1 (cited in repo README) | Perseus lexica and lemmata (incl. LSJ-derived) | Pass as a lemma list; not an analyser |

## Key findings

### Stanza `grc` (confirms ADR 0099, strengthens the reason)

- Stanza's resource index (`stanza-resources`, `resources_1.10.0.json`, `grc` entry) lists packages `perseus`, `perseus-diacritics`, `proiel`, `ptnk`, `default_fast`, `default_accurate`, `default`, and a language-level `pretrain` of `conll17` only.
- The `conll17` pretrain is the CoNLL 2017 word embeddings. LINDAT/CLARIAH-CZ record [hdl 11234/1-1989](http://hdl.handle.net/11234/1-1989) says the **CC BY-SA-NC 4.0 licence applies to the automatically generated annotations and word embeddings** (the record's own spelling; the licence file is "CC BY-NC-SA 4.0"). So the NC condition reaches every grc Stanza package through the embeddings, not only through the NC treebanks.
- The `perseus` and `proiel` packages are trained on NC UD treebanks (below). Only the `ptnk` package avoids a treebank NC input, and it still has the NC embedding.

### Treebank licences (primary sources)

- UD_Ancient_Greek-PROIEL `LICENSE.txt`: CC BY-NC-SA 4.0 (`UniversalDependencies/UD_Ancient_Greek-PROIEL`).
- UD_Ancient_Greek-Perseus `LICENSE.txt`: CC BY-NC-SA 2.5.
- UD_Ancient_Greek-PTNK: CC BY-SA 4.0 (`LICENSE.txt`); content is Septuagint (Codex Alexandrinus).
- PerseusDL `treebank_data` (AGDT 2.x repository) README: "Contents … licensed under CC BY-SA 3.0 US" with no NC statement. The cltk `greek_treebank_perseus` README states **AGDT 1.7 is CC BY-NC-SA 2.5**. Any input built from 1.7 is therefore NC; the v2.1 pin must be checked for that (below).

### Morpheus

- Output: lemma and morphological analysis per token in Perseus format, or database format with `-d`. Each word can return several parses; `-V` analyses verbs only. Nothing is disambiguated; no POS tagging; no sentence segmentation.
- Input: beta code by default (`perseids-tools/morpheus` README examples; `-k` keeps beta code). A polytonic Unicode reader therefore needs a conversion step, and that converter's licence is **not verified** here.
- The stemlib is in the same repository (`stemlib/Greek/derivs/ascii/*.asc`, plus `conjfile`). Its origin is not documented in the README. Lineage is unverified beyond the repository licences; the Perseus `greek_lexica_perseus` mirror (MPL 1.1) is a related lexicon but its link to the stemlib is not established.
- Last push to `perseids-tools/morpheus`: 2024-03-18. Repository size 8.59 MB.

### OGA `oga_src` v0.2.0

- Zenodo record [15199546](https://zenodo.org/records/15199546) (`mgbilby/oga_src`, version v0.2.0, published 2025-04-11): licence `cc-by-sa-4.0`; one CoNLL-U file per document; archive 761,489,280 bytes.
- The record states OGA's morphosyntax is "based on the Ancient Greek Dependency Treebank" and that OGA carries provisional authorship and date data.
- The GitHub repository has no detectable LICENSE file (`NOASSERTION`), so the Zenodo record is the licence evidence.
- ADR 0099 pins the single-file CoNLL-U packaging; the pin remains the accepted source.

## Unverifiable or open claims (stated plainly)

- **Accuracy on Attic prose:** no reported accuracy for Morpheus was located. The OdyCy and Stanza accuracies in their model cards are not usable as evidence because those models fail licence. The OGA annotation accuracy is not stated in the Zenodo record.
- **Morpheus stemlib lineage:** whether any stemlib data descends from NC-licensed material is not established. The upstream PerseusDL statement (BY-SA 3.0 US) and the fork's MPL-2.0 both lack NC.
- **AGDT version inside OGA:** the OGA record says "based on AGDT" but does not state the AGDT version. If any AGDT 1.7 material is present, that material is NC. This needs a check of the pinned file before any further use.
- **Diorisis lemmas:** the figshare corpus is CC BY 4.0, but the lemma method (TreeTagger with an NC-trained parameter file) does not establish that its lemmas are clean.
- **Dilemma:** not located in GitHub search or web search. No licence is verified; treat as absent.
- **TreeTagger licence:** the official page says "freely available for research, education and evaluation" and commercial use by agreement. It is not an NC CC licence, but it is not an open licence. Moot here, because the Greek parameter file fails on lineage.
- **Polytonic conversion:** no non-NC beta-code converter was verified.
- **Pinned data sizes:** Morpheus stemlib file sizes were not measured; OGA is 761,489,280 bytes as a zip, the CoNLL-U single-file size is in ADR 0099.

## Next steps (not performed here)

- Pin the Morpheus commit and run an offline Attic sample to measure parse multiplicity and coverage. That is a measurement, not an accuracy claim.
- Check the OGA pin for AGDT 1.7 provenance before reusing it beyond ADR 0099.
- Locate a verified non-NC Greek polytonic-to-beta converter, or write one and state its licence.
