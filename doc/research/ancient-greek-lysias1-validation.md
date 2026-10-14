# Lysias I (AGDT v2.1 ↔ canonical-greekLit) — candidate evidence, v2

Issue: #2046 · Parent decision: #2030 (stays open) · Probe: `scripts/validate_lysias1.py` (v2) and `scripts/check_dictionary.py` · Summary: `summary.json`

**Status: candidate evidence only.** Nothing here accepts a sentence, card, headword, annotation, or the Lysias I MVP. This v2 replaces the 10fcdb72 report's claims; see "Corrections" below.

## Corrections to 10fcdb72

- The canonical comparator is **not malformed**. `xml.etree.ElementTree` parses the pinned blob. The v1 probe used regular expressions, which is why it reported a problem.
- v1's self-joined spans were tautological. v2 checks every span against the canonical text itself (`verification_failures: 0`).
- v1 said "64 convention blocks". v2 classifies those tokens by exact mechanism: 50 elision-mark folds and 22 exact concatenations.
- v1's "5 rubric segments" and the two unexplained items (sentence 11, sentence 125) are reproduced. Sentence 11 is not reordered, and its binding is reported as a source-order defect.
- v1 had no lemma/UPOS accounting, no dictionary check, and no sentence contiguity check. These are now done (below).

## Pins and rights

| Input | Path | SHA-256 | Notes |
|---|---|---|---|
| AGDT v2.1 at `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` | `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` | `44ecf3a2…a568` | Rights: the root README of the treebank repo declares CC BY-SA 3.0 US for repository contents. The file has no per-file licence line; no per-file licence is claimed. |
| canonical-greekLit at `01b725d835e6e733062ffd79e0efdbae1ba06e5c` | `tlg0540.tlg001.perseus-grc2.xml` | `21dbae42…d6a` | TEI `licence` target CC BY-SA 4.0. |
| Kaikki Ancient Greek extract (dictionary check only) | `kaikki.org/dictionary/Ancient%20Greek/meaning/…` | not hashed (live pages) | The repo's attribution notice (`internal/dictionary/attribution.go`) says Wiktionary-derived, CC BY-SA 3.0 / GFDL, share-alike. The Kaikki index page has no licence line of its own. No gloss text is committed. |

These are attributed rights declarations, not legal certification.

## Method (v2)

1. **Canonical.** Parsed with ElementTree. The `edition` div holds 50 `section` divs with consecutive `n`. `note` (14) is skipped. `label` (5 rubrics: Νόμος ×3, Μάρτυρες ×2) is skipped and recorded. `del` (3 elements, 11 chars) and `add` (3 elements, 8 chars) are kept as printed text and flagged. `q` (11 elements, 1,187 chars) is kept and flagged. One `gap reason="lost"` appears in the section divs. Whitespace is collapsed. Running text: 15,110 code points, NFC.
2. **AGDT.** 2,802 printed nodes. 32 artificial nodes that carry a form field are excluded because they are not printed occurrences. Punctuation nodes (374) carry no lexical key.
3. **Binding.** Three levels, with no reordering and no re-spelling:
   - Level 1: exact NFC word keys (`difflib`, no autojunk).
   - Level 2: within unequal blocks, elision/breathing fold, treating U+0313 as the elision mark. Only the fold class is labelled "fold".
   - Level 3: exact concatenation of consecutive AGDT nodes equal to adjacent canonical units. Each node's span is cut from the canonical text by cumulative length and verified there.
4. **Sentences.** Native segments are checked for contiguity: punctuation between bound tokens, and leading/trailing punctuation, must match the canonical text exactly after the elision fold. Completeness is not checked (see blockers).
5. **Vocabulary.** Candidate UPOS from the TAGSETS letter names (n NOUN, a ADJ, v VERB, d ADV, l DET, p PRON, r ADP, c CCONJ, m NUM, i INTJ, g PART, u PUNCT, x X). AuxV gives AUX, and AuxC gives SCONJ. Content is the contentPOS rule in `internal/enrichment/lexical.go` (not PUNCT, SYM, SPACE, X, DET, AUX, PRON). Identity key: canonical lemma (NFC, lowercase, final sigma folded) plus candidate UPOS. The AGDT lemmas have no homograph discriminator. Selection is ≥3 content occurrences in this Book (provisional).

## Results

### Alignment

| Class | Tokens |
|---|---|
| exact (1:1) | 2,346 |
| fold (U+0313 ↔ ʼ, elision; includes 5 non-NFC `παῤ` ↔ `παρʼ`) | 50 |
| concat_exact (e.g. `κ-`+`ἀγὼ` ↔ `κἀγὼ`) | 22 |
| unbound | 10 |

Keyed tokens: 2,428. Canonical word units: 2,410, of which 3 are unbound. Verification failures: 0.

Unbound blocks: 8.

- 5 × AGDT-only rubric labels (native segments 78, 82, 84, 90, 121; 1-based).
- 2 × `replace_block`, sentence 11 (1-based): AGDT nodes 25–26 (`-τε μή`) and 28–29 (`-τε μή`) against two canonical `μήτε` units. The concatenation test fails (`τεμή` ≠ `μήτε`), so no span is bound. Nothing is reordered. No pinned document states an enclitic or node-order convention: a restricted search of the pinned guidelines and READMEs for crasis, elision, hyphen, enclitic, and clitic found only the NFC note on elliptical nodes.
- 1 × `replace_1to1`, sentence 125: node 15 `οὔτε` against canonical `οὕτε`. Accent variant, no convention found.

### Native sentences (134)

- 126 verified: contiguous, punctuation exact, all tokens bound. "Verified" does not mean complete.
- 8 problem: sentence 11 (4 unbound), sentence 125 (1 unbound), 5 rubric-only segments (78, 82, 84, 90, 121), and sentence 2 (1 trailing-punctuation mismatch).
- Sentence 2 (1-based) ends in `·` after `εἴη`, but the canonical text has no mark there. Sentence 3 starts `ὅστις…` right after `εἴη` with no terminal before it. Canonical punctuation does not support the native boundary between sentences 2 and 3. This is the clearest evidence that native segment labels cannot establish completeness.
- Medial terminal inside a segment: 1 (sentence 118).
- Units (whitespace tokens): median 15. Over 50 units: sentences 8 (65 units, 390 code points), 44 (52), 52 (58), 94 (58). Over 400 code points: 0.

### Vocabulary accounting (every printed node)

- Content (contentPOS): 1,856 nodes. 1,846 bound (exact, fold, or concat). 10 unbound: rubric nouns 5, sentence 11 nodes 4, sentence 125 `οὔτε` 1.
- Non-content: 946 nodes, all word tokens bound. PUNCT 374 (unkeyed), PRON 287, DET 284, AUX 1.
- Total: 1,856 + 946 = 2,802.
- Distinct identities: 548. Selected (≥3): 163, of which 137 are `card_candidate` and 26 are `upos_unresolved`.
- The 26 unresolved identities are 20 ADV (from letter `d`, adverb/particle ambiguous in the pinned guidelines) and 6 CCONJ (from letter `c` without AuxC). All 26 have at least one eligible occurrence, so UPOS is the only thing blocking them.
- Occurrence-level reasons across all content occurrences: over-cap 173, sentence unverified 78, unbound 10, editorial del/add 3. An occurrence can carry more than one reason.

### Headwords and dictionary (exact raw lemma strings, 158)

- 153 have a Kaikki page (200) for the exact string.
- `δὲ` and `καὶ` (grave in AGDT) resolve at `δέ` and `καί` under NFD acute. This is a headword-spelling decision. The canonical-lemma profile does not fold accents, so the lemma keys are left unchanged. Both are also UPOS-unresolved (CCONJ, ADV).
- `Νόμος` (capital, rubric lemma) has no page; `νόμος` does. The identity `νόμοσ|NOUN`, one of the 137 candidates, mixes both raw forms.
- `εἴσειμι` (6 occurrences) and `συνδειπνέω` (4) have no page under exact, NFD-acute, or folder variants. Both are measured as absent from this extract. Both are `card_candidate` identities, so the headword needs a dictionary decision before any card.

### Card candidates

137 identities have an earliest occurrence that passes every check above (bound, verified sentence, within caps, no editorial token, determinate UPOS). This is candidate evidence only. Completeness of their sentences has not been established.

## Verified now vs conditional

**Verified by exact text on the pinned inputs:** both pins; ElementTree parse; 50 sections; 2,802 printed spans checked against canonical text (0 failures); 2,346 exact, 50 fold, and 22 concatenation bindings; 10 unbound tokens with named causes; contiguity of 126 segments; dictionary exact-string results.

**Conditional:**
- UPOS candidates. No reviewed AGDT→UPOS table exists in the repo or the pinned docs; the mapping uses TAGSETS names and relation labels.
- Identity key. The #2033 profile is applied; there is no homograph discriminator in the source.
- Fold and concatenation classes. These rest on exact text equality only; no pinned convention documents them.
- Selection threshold 3 (provisional).
- Sentence completeness. Not established for any sentence.

## Remaining blockers (named)

1. **UPOS decision.** `d` (ADV vs PART) and `c` without AuxC (CCONJ vs SCONJ) are unresolved in the pinned guidelines. These block 26 identities (20 ADV, 6 CCONJ). The narrowest decision: accept a candidate default for those letters, or leave them blocked.
2. **Sentence completeness.** No reviewed segmentation convention exists. Canonical punctuation contradicts the native boundary at sentences 2/3. No sentence can be certified complete until a convention is chosen.
3. **Source defects.** Sentence 11 (`-τε μή` order against `μήτε`) and sentence 125 (`οὔτε` vs `οὕτε`) remain unbound. These are source-level differences, not something the probe resolves. Other occurrences are unaffected.
4. **Over-cap.** Four native segments exceed 50 units. The cap decision (split, or reject) is not made.
5. **Headword spelling.** `δὲ`/`καὶ` grave vs acute; `Νόμος` capitalization in the rubric; two candidate headwords (`εἴσειμι`, `συνδειπνέω`) with no dictionary page.
6. **Rights.** No per-file licence on the AGDT file. The root README declares CC BY-SA 3.0 US for repository contents. The dictionary is share-alike. This is an attributed declaration, not legal certification.

Not performed: a printed Lamb 1930 edition check (not part of the reopened checks); any provider call, NLP, Steadman, or outreach.

## Reproduction

```sh
sh doc/research/ancient-greek-lysias1-validation/scripts/fetch_inputs.sh <inputs_dir>
python3 -I doc/research/ancient-greek-lysias1-validation/scripts/validate_lysias1.py <inputs_dir> <detail_dir> doc/research/ancient-greek-lysias1-validation
python3 -I doc/research/ancient-greek-lysias1-validation/scripts/check_dictionary.py <detail_dir>
python3 -I doc/research/ancient-greek-lysias1-validation/scripts/validate_lysias1.py <inputs_dir> <detail_dir> doc/research/ancient-greek-lysias1-validation   # rerun to add dictionary counts
```

Per-token, per-identity, and per-sentence detail stays in `<detail_dir>` (scratch) because of the output-specific rights question (#2035).
