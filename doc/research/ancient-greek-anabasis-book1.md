# Xenophon, *Anabasis* Book 1 as a Greek MVP candidate

**Research date:** 2026-10-14

**Issue:** [#2042](https://github.com/justin-hayes/mouseion/issues/2042) (map [#2021](https://github.com/justin-hayes/mouseion/issues/2021), parent [#2030](https://github.com/justin-hayes/mouseion/issues/2030)).

**Scope:** Offline, reproducible measurement of one text pairing for Book 1 only: the Gorman
treebank (upstream commit `d823e96d`) against the canonical-greekLit TEI text (commit
`01b725d8`). No provider calls, no card generation, no rights-holder or author contact, no
gated or rejected source access, no production code, dependency, ADR or glossary changes.
Books 2–7 and the Steadman companion readers are outside this measurement. The fallback works
(Lysias I, Plato's *Crito*) are not evaluated.

**Status:** The bounded investigation is complete. Full exact-eligible measurement is **not
established**. Book 1 is **not accepted** as the Greek MVP text. This document states the
blockers and the decision frontier; it does not claim acceptance, and it does not treat
candidate-level similarity as proof of unit correspondence.

Reproduce with [`scripts/run_all.sh`](ancient-greek-anabasis-book1/scripts/run_all.sh). The
per-run JSON summaries are committed in
[`ancient-greek-anabasis-book1/summaries/`](ancient-greek-anabasis-book1/summaries/).

## Executive findings

1. **Three labelled text routes were measured, not one chosen edition.** Route labels are used
   throughout: **P** (sic kept, deletions kept), **R** (corrections used, deletions omitted),
   **H** (corrections used, deletions kept). None is a selected edition or binding. Section 2
   explains each label's derivation and defects.
2. **Strict unit alignment is high but not decisive.** Route H: 10,285 of 10,495 units exact
   (98.0%); 202 variant pairs (193 are the elision-glyph case, not accepted); 8 unmatched.
   Routes P and H differ by one exact unit. Route R has 81 unmatched units and 200 variants.
3. **Sentence completeness is the main blocker.** Of 440 Gorman sentences whose units are all
   exact in route H, only **159** agree with a canonical punctuation candidate at both
   boundaries. The remaining 280 mismatch at one or both boundaries, mostly at the ano teleia
   (`·`; 279 of the 280), which the candidate rule treats as medial. Gorman's segmentation and the canonical
   punctuation rule are two unvalidated segmentations; agreement is evidence, not proof.
4. **Identity counts are conditional.** 493 identities have at least three nodes. Whether a
   selected identity has a qualifying occurrence depends on an unresolved UPOS mapping for
   `c`, `p`, and `εἰμί`. Under the candidate-level diagnostic: 438 identities. Under the
   corroboration rule: **369**. Neither is final card coverage.
5. **Sensitivities move the numbers, not the verdict.** Accepting the elision-glyph variant
   would raise route H's exact units to 10,478 and corroborated sentences to 222. Ignoring
   the POS ambiguity raises the corroborated identity count to 405. Neither is accepted.
6. **Licence is unresolved.** The Gorman README declares CC BY-NC-SA 4.0, while the bundled
   `TREEBANK_LICENSE` file is BY-SA 4.0 legal text. The canonical TEI declares CC BY-SA 4.0.
   This conflict is not resolved here, and the author was not contacted.
7. **Dictionary evidence was not measured.** This is "not measured", not "absent". No lexical
   resource was loaded or tested.

## 1. Sources and pins

| Source | Pin | Files | SHA256 |
|---|---|---|---|
| Gorman treebank | `vgorman1/Greek-Dependency-Trees` @ `d823e96d253277bc1872b83ff5ac8c393a0f830a` | `xml versions/Xen_Anab_book_1.1-5.xml` | `6b6b9c3c2c9c51f5d2e60f73f6cf8e07d8e620b7c13582aa5f8044889f0d4d40` |
| | | `xml versions/Xen_Anab_book_1.6-9.xml` | `8091055396cc74d7de24c30e2aa13ea4665ca3110b92d2d51d76e013bea9ab6c` |
| Canonical TEI | `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c` (committer date 2026-10-06) | `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml` | `5bf59352cab3dec763ecff9f4061385b8307146d2501432a5f5b178d6f31a4c2` |

Fetch commands (the pinned files were re-downloaded and matched these SHA256 values):

```sh
gh api "repos/vgorman1/Greek-Dependency-Trees/contents/xml%20versions/Xen_Anab_book_1.1-5.xml?ref=d823e96d253277bc1872b83ff5ac8c393a0f830a" --jq .content | base64 -d
gh api "repos/vgorman1/Greek-Dependency-Trees/contents/xml%20versions/Xen_Anab_book_1.6-9.xml?ref=d823e96d253277bc1872b83ff5ac8c393a0f830a" --jq .content | base64 -d
gh api "repos/PerseusDL/canonical-greekLit/contents/data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml?ref=01b725d835e6e733062ffd79e0efdbae1ba06e5c" --jq .content | base64 -d
```

The raw files are kept in scratch and are **not committed**: they are upstream treebank and
TEI text under licences that are unresolved (section 7). Also not committed: the generated
unit-level and candidate-level tables, which contain source text. Only counts and short lists
are committed.

**Canonical source metadata** (TEI header, read in this run): title *Ἀνάβασις*; editor
Edgar Carew Marchant; `sourceDesc` cites Marchant's *Opera Omnia* vol. 3, Clarendon Press,
Oxford, 1904 (reprint 1961); Perseus 2.0 release 1996; licence CC BY-SA 4.0. The
`<choice>`, `<sic>`, `<corr>`, `<del>`, and `<add>` markup is Perseus's encoding. It is not
independently verified against the 1904 page.

**Gorman metadata:** `<treebank format="aldt" version="1.5">`; `<date>` 2022-07-15; Book 1
sentence counts 342 (file 1.1–5) and 269 (file 1.6–9), total **611**. The file headers name
annotation tools (Arethusa, segtok) and do not name a source edition for the printed text.
**Gorman's source edition is therefore unknown.** The README says trees uploaded after
January 2020 were pre-parsed at Pedalion and hand-corrected by V. Gorman. The `ref` attributes
on `<word>` (for example `8995993`) do not resolve against the pinned canonical file (zero
matches for a sampled value), so they are not used.

## 2. Authority routes: derivation, versions, offsets, defects

All three routes are produced by `scripts/build_canonical.py` from the same TEI `<p>` content,
section by section (`1.1.1`–`1.10.19`, 205 sections). `<milestone>` elements are dropped in
every route. Other element text is kept unless stated. Offsets are Unicode code points into
the route string, with each section followed by one space; the string is right-stripped.
Offsets are relative to the route string, not to any printed page.

| Label | Deletion (`<del>`) | `<choice>` content | Code points | Tokens (word + punct) |
|---|---|---|---|---|
| **P** (`printed`) | kept as encoded | `<sic>` (the `<corr>` is dropped) | 58,951 | 10,488 (9,321 + 1,167) |
| **R** (`reading`) | omitted | `<corr>` | 58,560 | 10,415 (9,258 + 1,157) |
| **H** (`hybrid`) | kept as encoded | `<corr>` (the `<sic>` is dropped) | 58,950 | 10,488 (9,321 + 1,167) |

Raw character data (all children kept, so `<sic>` and `<corr>` both present) totals 58,749 code
points (`out/build_canonical.log`). It is not an authority route and is not used.

**Route H, the critical review.** The earlier label "hybrid reading variant" was misleading.
Route H is not a human-selected authority, not a published edition, and not an accepted silent
merge. Its definition is: keep the deleted text the TEI encodes, but replace each `<choice>`
with its `<corr>` form. That combination does not correspond to a documented edition or
binding in the source. Specific defects:

- **Selection is circular.** H was preferred because it gives the most exact units against
  Gorman's text. The exact counts are therefore optimistic. They are not independent validation
  of H, and the same is true of any route chosen on this basis.
- **Routes P and H are not separated by this evidence.** They differ in code points by one, and
  in exact units by one (10,284 vs 10,285).
- **Deletions and corrections are editorial, not decided.** The TEI marks them. Nothing in this
  run decides whether a deletion is part of the text a learner should read.
- **Offsets are internal.** The retained offsets are per-section in the route string, carried
  into `units_<route>.tsv` as `canon_start_cp`/`canon_end_cp`. They do not map to a printed
  page.

**Route R** differs from H by 390 code points (the deleted text). 64 Gorman word units are
unmatched in R as "gorman-only". 62 of those 64 have an identical key somewhere in route H or
P, which is consistent with Gorman encoding deleted material. This was not verified per unit,
so it is not a finding.

## 3. Denominators (route H, strict)

| Quantity | Count | Derivation |
|---|---|---|
| Gorman `<word>` nodes (all) | 10,674 | 5,704 (file 1.1–5) + 4,970 (file 1.6–9) |
| Artificial (elliptic) nodes excluded | 139 | `artificial="elliptic"` nodes, not in printed text |
| Printed nodes | 10,535 | 10,674 − 139 |
| Clitic merges (leading-hyphen nodes) | 40 | merged into the preceding unit; nodes keep their own lemma and POS |
| **Units** | **10,495** | 10,535 − 40 |
| Units, punctuation | 1,169 | all nodes tagged `u` |
| Units, word | 9,326 | 10,495 − 1,169 |
| Word units exact | 9,119 | of 9,326 |
| Trailing-hyphen (crasis) pieces | 5 | each followed by an unmatched unit (`no-canonical-counterpart-in-block`) |
| Unit status: exact / variant / unmatched | 10,285 / 202 / 8 | sums to 10,495 |
| Canonical tokens | 10,488 | 9,321 word + 1,167 punctuation |
| Canonical whitespace units | 9,321 | `text.split()` |
| Canonical code points | 58,950 | route string, right-stripped |

**The discrepancy:** 10,674 source nodes vs 10,285 + 202 + 8 units. Reconciled exactly:
10,674 − 139 (artificial) = 10,535 printed nodes; 10,535 − 40 (clitic merges) = 10,495 units =
10,285 + 202 + 8. The units are the comparison denominator, not the nodes.

## 4. Strict results per route

| Route | Units exact / variant / unmatched | Candidates (full exact) | Candidates corroborated | Native sentences all-exact | Identities selected (≥3) | Qualifying (candidate diagnostic) | Qualifying (corroborated) |
|---|---|---|---|---|---|---|---|
| P | 10,284 / 203 / 8 | 382 (243) | 159 | 439 | 493 | 438 | 369 |
| R | 10,214 / 200 / 81 | 381 (243) | 158 | 433 | 493 | 438 | 369 |
| H | 10,285 / 202 / 8 | 382 (243) | 159 | 440 | 493 | 438 | 369 |

"Candidates" are canonical sentences cut at `. ; ! ?`. Ano teleia (`·`) is medial by this rule
(see section 5). "Full exact" means every canonical token in the candidate is matched by an
exact unit. "Corroborated" means at least one Gorman sentence has all units exact and its
first and last canonical tokens match that candidate's. Candidate length: 22 candidates exceed
50 whitespace units and 4 exceed 400 code points in H. None of the 159 corroborated sentences
exceeds either bound.

### Sensitivities (diagnostic only; none is accepted)

| Route | Flags | Units exact / variant / unmatched | Candidates full exact | Corroborated candidates | Native all-exact | Qualifying (candidate) | Qualifying (corroborated) |
|---|---|---|---|---|---|---|---|
| P | glyph-ok | 10,477 / 10 / 8 | 372 | 222 | 599 | 451 | 406 |
| P | ambig-ignored | 10,284 / 203 / 8 | 243 | 159 | 439 | 477 | 405 |
| P | both | 10,477 / 10 / 8 | 372 | 222 | 599 | 491 | 444 |
| R | glyph-ok | 10,405 / 9 / 81 | 372 | 221 | 591 | 451 | 406 |
| R | ambig-ignored | 10,214 / 200 / 81 | 243 | 158 | 433 | 477 | 405 |
| R | both | 10,405 / 9 / 81 | 372 | 221 | 591 | 491 | 444 |
| H | glyph-ok | 10,478 / 9 / 8 | 373 | 222 | 600 | 451 | 406 |
| H | ambig-ignored | 10,285 / 202 / 8 | 243 | 159 | 440 | 477 | 405 |
| H | both | 10,478 / 9 / 8 | 373 | 222 | 600 | 491 | 444 |

- **glyph-ok** treats U+2019 vs U+02BC elision glyphs as aligned. This is not accepted under the
  normalization decision. It is reported as sensitivity only, not as eligibility.
- **ambig-ignored** ignores the `c`/`p`/`εἰμί` POS ambiguity when deciding qualification. It does
  not change the identity keys, so the 493 count stays fixed. The POS ambiguity is unresolved.

## 5. Sentence completeness (independent corroboration)

Route H, strict (611 Gorman sentences):

- 171 sentences contain at least one non-exact unit and are excluded.
- 440 sentences have all units exact. Of these:
  - 280 have a boundary mismatch: 118 with the start equal and the end different, 112 with the
    start different and the end equal, 50 with both different;
  - 1 is not matched to a fully exact candidate;
  - **159 are corroborated** (exact units, one candidate, boundaries equal, candidate fully
    exact).

Every exact Gorman sentence lies inside one canonical candidate (no exact sentence spans two
candidates). The mismatches are therefore Gorman sentences that are fragments of, or shifted
within, a candidate.

**Main cause, measured over the 280 mismatches only:**

| Boundary pattern | Count |
|---|---|
| Start equals candidate; Gorman sentence ends at `·` (canonical ano teleia) | 118 |
| Gorman sentence starts after `·`; end equals candidate | 111 |
| Starts after `·` and ends at `·` | 50 |
| Starts after another token (not `·`); end equals candidate | 1 |

So 279 of 280 mismatches are at an ano teleia: Gorman segments there, and the candidate rule
treats `·` as medial. Whether `·` ends a sentence is therefore an open policy question
(section 8), not a matcher error. The one remaining case was not inspected.

**Candidate-side accounting:** of 382 candidates, 243 are fully exact by the probe. Of those,
**159 are corroborated** and **84 are not**. The 84 are exact at the token level only; no
Gorman sentence confirms their boundaries. They must not be treated as complete sentences.

## 6. Identity selection (conditional)

- 1,496 identity keys (fold of lemma headword, UPOS proposal, homograph digit).
- The UPOS proposal is not resolved. `c` maps to `CCONJ|SCONJ`, `p` to `PRON|DET`, and `εἰμί`
  is flagged AUX-vs-VERB. These are placeholders, not resolved tags.
- 493 identities have at least three nodes (8,127 occurrences). The three-node threshold is a
  provisional baseline for one Book, not the 3/15 rule over the whole corpus.
- Qualifying conditions (probe logic): unit exact; candidate fully exact; terminal punctuation;
  at most 50 whitespace units and 400 code points; a headword present; POS not ambiguous. The
  corroborated count adds: the candidate must be corroborated.

Route H: 438 identities with a candidate-level qualifying occurrence; **369** with a
corroborated one. These are diagnostic figures, not card coverage. Card coverage also needs the
POS decision, the glyph decision, the boundary policy, and a licence decision.

## 7. Licence, provenance, and version status

- **Gorman README** (pinned): "licensed under a Creative Commons Attribution-NonCommercial-
  ShareAlike 4.0 International License (see TREEBANK_LICENSE file)."
- **`TREEBANK_LICENSE`** (pinned): BY-SA 4.0 legal text. It contains no NonCommercial clause.
- **Canonical TEI:** CC BY-SA 4.0.
- Conflict status: **unresolved.** Whether any use of the Gorman text is permitted by the
  NC clause, as the README says, or only by the BY-SA text the file contains, is not decided
  here. No permission is inferred, and the most restrictive reading is not adopted as a fix.
- Gorman's Book 1 files were first committed 2022-07-24 and last changed 2022-08-14 (upstream
  history). The README's version 1.0.1 (2019) predates these files, so the pinned commit is
  the version used.
- The guidelines PDF text extraction is partial. No homograph-numbering definition was found,
  so the meaning of the `ἤ1`/`λέγω3`-style digits is not established. The digits are kept as
  part of the identity key and are not interpreted.

## 8. Assumptions, defects, and decision frontier

**Assumptions (each is a stated choice, not a fact):**

- Route labels P/R/H are analysis constructs, not editions (section 2).
- Clitic pieces beginning with `-` are merged into the preceding word. Trailing-hyphen crasis
  pieces are not merged.
- Candidate sentences end at `. ; ! ?`. Canonical `;` is also used as a Greek question mark
  (9 instances in Book 1), so some candidate boundaries may be wrong.
- Ano teleia `·` is medial. Gorman's sentence boundaries disagree with this at 279 mismatch
  boundaries (section 5).
- Unit keys use NFC, Unicode lowercase, and final-sigma folding. Punctuation uses NFC only.
- Positional pairing inside a replace block is labelled "variant" for the pair. It is not
  evidence that the two tokens are the same word.

**Defects:**

- Route selection is circular (section 2).
- The Gorman source edition is unknown; the mismatch may come from Gorman's text.
- 193 elision-glyph variants (U+2019 vs U+02BC) are not resolved. No blanket equivalence is
  accepted.
- The crasis `τ-` + `ἔμπαλιν` (1.4.15) is split by Gorman into two units. The canonical token
  is `τοὔμπαλιν`. This gives 5 partial-piece cases and 5 unmatched successors.
- Textual differences in the canonical and Gorman texts: 1.1.8 `Τισσαφέρνης` vs `Τισσαφέρνους`;
  1.5.9 `ἡ ἀρχὴ` vs `τῇ ἀρχῇ`; Gorman-only `ἑπτὰ` (1.2.5) and a comma (1.1.7 and 1.10.12);
  canonical-only `ἀνέβη` (1.1.2).
- Sentence-final `;` as a question mark (section 4 assumptions).

**Decision frontier (each must be decided by a human; none is decided here):**

1. Which canonical text, or which printed source, should be the authority. P, R, and H are not
   separated by this evidence, and the Gorman source edition is unknown.
2. Whether `·` ends a sentence, and whether `;` as question mark ends one.
3. The UPOS mapping for `c`, `p`, and `εἰμί`, under a reviewed rule.
4. The elision-glyph normalization rule (accepted only by a reviewed producer rule).
5. The crasis and clitic unit policy.
6. The licence (NC clause vs BY-SA text). No author or provider was contacted.
7. Dictionary evidence: not measured.
8. Books 2–7: at the pinned commit, Gorman has only Book 3 (`Xen_Anab_book_3.xml`, 1,002,135
   bytes, not measured) and no Book 2 or 4–7 file. The canonical TEI has all seven books.

## 9. Human-review examples

Short, source-grounded examples only; each is one token or a short fragment, cited by Gorman
file, sentence id, and canonical section. Quoting more would pass the unresolved licence
question.

| Case | Gorman | Canonical (route H) | Where | Status |
|---|---|---|---|---|
| Elision glyph | `δ’` (U+2019) | `δʼ` (U+02BC) | 1–5, sentence 9, 1.1.4 | variant, not accepted |
| Crasis split | `τ-` + `ἔμπαλιν` | `τοὔμπαλιν` | 1–5, sentence 267, 1.4.15 | partial-piece, unmatched |
| Different token | `Τισσαφέρνης` | `Τισσαφέρνους` | 1–5, sentence 23, 1.1.8 | variant |
| Different token | `ἡ` / `ἀρχὴ` | `τῇ` / `ἀρχῇ` | 1–5, sentence 317, 1.5.9 | variant |
| Gorman-only | `ἑπτὰ` | (none) | 1–5, sentence 52, 1.2.5 | unmatched |
| Canonical-only | (none) | `ἀνέβη` | 1.1.2 | canonical-only |
| Punctuation | `·` | `,` | 6–9, sentence 248, 1.10.12 | variant |

## 10. Runtime and sizes

- `run_all.sh` (build, 12 probe runs, 12 completeness runs): **8.6 s** wall time on this machine,
  run twice with identical summaries.
- Single probe run (route H, strict): about 0.65 s. Completeness: about 0.07 s.
- Scratch sizes: raw upstream files 4.1 MB; generated tables 22 MB (not committed).
- Committed summaries: 24 JSON files, about 100 KB.

## 11. Reproduction

```sh
export WORK="$HOME/.cache/scratch/greek-2030-anabasis-validation"   # holds raw/ with the pinned files
doc/research/ancient-greek-anabasis-book1/scripts/run_all.sh
```

The script is deterministic: it was run twice, and the summary files matched byte for byte. The
probe's identity ordering uses an explicit tie-break for reproducibility. No script calls the
network or an LLM.
