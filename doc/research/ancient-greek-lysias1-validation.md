# Lysias I (On the Murder of Eratosthenes) — AGDT v2.1 validation probe

Research ticket: #2046. Map: #2021. Parent: #2030.
Status: **candidate evidence, not acceptance.** Nothing here approves Lysias I as the Greek MVP source.

## Verdict

- The pinned AGDT v2.1 file and the pinned canonical comparator both match their pinned SHA-256 hashes. The canonical file has 50 consecutive sections. The AGDT file has no section labels, so its 50-section scope is supported only by the token alignment and matching first and last tokens, not by a section-level check.
- At exact whitespace-token level the two agree on 2,337 tokens. There are 71 non-equal blocks, and each is classified (below). Two blocks are real textual or tokenization variants that no rule explains. Those two stay unresolved.
- The 64 "convention" blocks depend on a reviewed folding rule (elision mark, clitic hyphen, punctuation placement). That rule is not yet a human-approved convention, so exact-text agreement is not established.
- Vocabulary, sentence completeness, rights, and dictionary availability are **not** ready to certify. See Blockers.

## Pinned inputs and reproduction

| Input | Pin | SHA-256 |
|---|---|---|
| AGDT v2.1 `tlg0540.tlg001.perseus-grc1.tb.xml` | `PerseusDL/treebank_data@bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` | `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568` |
| Canonical `tlg0540.tlg001.perseus-grc2.xml` | `PerseusDL/canonical-greekLit@01b725d835e6e733062ffd79e0efdbae1ba06e5c` | `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a` |

- Fetch and verify: `scripts/fetch_inputs.sh <dest>` (bare partial clones, `git show` of the pinned blobs, then `sha256sum -c`). Verified in a fresh directory: both hashes OK.
- Probe: `python3 -I scripts/validate_lysias1.py <inputs> <detail_dir> <committed_dir>`. Stdlib only. Probe runtime 0.04 s. Tool: Python 3.13.5 on Linux 6.12. Probe script SHA-256 is recorded in `summary.json`.
- Committed outputs: this report and `ancient-greek-lysias1-validation/summary.json` (counts, pins, and two short diagnostic token pairs). Per-token, per-lemma, and full-text tables stay in scratch under the output-specific rights question (#2035). Scratch detail totals about 300 KB.
- Reconstruction version: `lysias1-recon-v1`.

## Authority and extraction (what is what)

- **Text authority:** not established. The AGDT file is the annotation carrier and declares its own edition (Lamb 1930 in `monogr`) but does not verify its digital text. The canonical file declares Lamb 1930 and CC BY-SA 4.0 in its licence line.
- **Extraction/reconstruction:** `lysias1-recon-v1` joins printed AGDT forms with spaces, except that punctuation and hyphen clitics attach without a space. Every printed occurrence was checked by exact code-point slicing (`text[start:end] == form`), with 0 failures.
- **Comparator:** canonical section text, with `<note>` apparatus and `<label>` rubrics removed. Tags are stripped, not parsed. The file has unclosed `<div>` elements, so XML parsing was not used.
- **Not checked:** the printed Lamb 1930 edition. The header claim is recorded as declared, not verified.

## Verified (deterministic, reproducible)

| Measure | Value |
|---|---|
| AGDT native sentences | 134 (none empty) |
| AGDT nodes | 2,834 |
| Artificial nodes (not printed) | 32 (23 empty `[n]`, 9 elided with form) |
| Printed occurrences | 2,802 (all span-checked) |
| Canonical sections | 50, numbered consecutively 1–50 |
| Canonical editorial elements | `note` 14, `del` 3, `add` 3, `gap` 3, `label` 5 (rubrics) |
| Reconstructed text | 15,167 code points, 2,415 whitespace tokens |
| Canonical running text | 2,409 whitespace tokens |
| Exactly equal tokens | 2,337 (AGDT 96.8%, canonical 97.0%) |
| Non-equal blocks | 71 (66 replace, 5 delete) |
| First and last token | agree (`περὶ` … `ἐπειθόμην.`). Endpoints alone are not proof. |

Non-equal blocks, by rule-based classification (`scripts/validate_lysias1.py`, `classify_block`):

- **64 convention-type**: elision mark U+0313 in AGDT vs ʼ (U+02BC) in comparator (e.g. `δ̓` vs `δʼ`), clitic/crasis hyphens (`κ-ἀγὼ` vs `κἀγὼ`), and punctuation placement. Classified only by equal letters after folding; not adjudicated.
- **5 rubric-only**: AGDT includes `Νόμος.`/`Μάρτυρες.` as text. The comparator carries them as `<label>`.
- **2 unexplained**:
  1. Sentence 11: AGDT `ὥστε-τε μή λυπεῖν-τε μή` vs comparator `ὥστε μήτε λυπεῖν μήτε`. AGDT splits μήτε into μή + τε, which changes lexical identity.
  2. Sentence 125: AGDT `οὔτε` vs comparator `οὕτε` (a real accent/word variant).

Five AGDT forms are not NFC-normalized. Five replace blocks pair `παῤ`-type AGDT forms with the comparator's `παρʼ`. They sit inside the convention-type blocks.

Editorial and extraction defects: canonical `<del>`/`<add>` text is kept in the running text, and `<gap>` is counted but not reconstructed. The canonical file contains malformed nesting, which the stripped-tag extraction tolerates.

## Conditional diagnostics (not a verified snapshot)

These depend on the unperformed canonical lemma-to-headword mapping. They are conditional.

- Lexical occurrences (POS `n`, `v`, `a`, `d` as tagset letters): 1,481. None unresolved (no missing or `undefined` lemma among printed nodes).
- Raw AGDT lemma identities: 502. Homograph digits: none. `.R`/`.I` notation: none. Both absent, not unknown.
- Selection under the provisional rule (≥3 lexical occurrences in this single Book): 136 identities, 1,017 occurrences.
- Unaligned selected occurrences: 36, across 15 identities (δέ 10, καί 6, ἀληθής 3, τε 3, νόμος 2, μή 2, μάρτυς 2, and one each for ἄλλος, ἀνήρ, οὔτε, οὐ, οἶδα, εἴσειμι, εἰμί, δέω). Most are elision-mark blocks. Every selected identity still has an aligned occurrence, so no identity is missing from the alignment.
- Rubric tokens `νόμος` and `μάρτυς` enter the lexical count from AGDT (they are AGDT nouns), which is a rubric-inflation risk. Not removed here.
- AuxV: 1 token, `ἐστίν` (`v3spia---`, lemma `εἰμί`), which is a conditional AUX case. Other Aux relations are counted as raw relations (`AuxP` 138, `AuxY` 195, `AuxX` 231, `AuxZ` 92, `AuxK` 135, `AuxC` 91, `AuxG` 4).
- Raw POS letters: `v` 569, `d` 341, `p` 287, `l` 284, `n` 328, `a` 243, `c` 215, `r` 136, `i` 25, `u` 374. The letter-to-UPOS mapping is not applied here.

## Sentences (native segmentation; not certified)

- 134 native segments, all non-empty. Units (whitespace-delimited, after trim): min 1, median 15, p90 35, max 65. Code points: min 6, median 95, p90 226, max 390.
- Over the provisional caps: 4 sentences over 50 units (native sentences 8, 44, 52, 94); 0 over 400 code points.
- Terminal marks: `.` 86, `·` (U+00B7) 42, `;` 6. Greek `;` is a question mark in these sentences, and one further `;` is medial.
- Five rubric-only native segments (`Νόμος.`, `Μάρτυρες.`) are one-unit sentences: native sentences 78, 82, 84, 90, 121.
- Completeness is not certified. Native boundaries were reported, not validated. No regrouping was attempted.

## Rights (attributed claims, not legal certification)

- AGDT root `README`: content licensed under CC BY-SA 3.0 US (line 15). Its third-party-rights qualification was not separately verified.
- AGDT v2.1 Greek `README` and the tb.xml file header: the tb.xml header carries no licence line. The v2.1 README's manual-annotation and Morpheus statements were not separately re-verified in this unit.
- Greek guidelines conflict: the pinned 2.0 guidelines allow AuxK terminals of period, semicolon or ano teleia. The ADR convention prefers medial ano teleia. The 1.1 guidelines (the file's declared scheme) enumerate no terminal-mark list. This is recorded as a documented ambiguity.
- v1.x NC terms are not applied by association. The pinned v1 guidelines are cited only for the annotation scheme.
- Human annotation does not require a second reviewer under this probe.

## Dictionary availability

- **Not measured.** No licensed or reviewed grc lexical evidence was checked for local or public availability in this unit.

## Human-review example pointers (sourced, not approved)

Sentences are given by native AGDT `sentence id`. None is marked reviewed.

- Clitic/tokenization variant: sentence 11 (`ὥστε-τε μή`, `λυπεῖν-τε μή`).
- Accent/word variant: sentence 125 (`οὔτε`/`οὕτε`).
- Crasis hyphen, convention-type: sentence 30 (`κ-ἀγὼ`), sentence 31 (`κ-ἀγὼ`), sentence 67 (`κ-ἀγὼ`).
- Elision mark, convention-type: e.g. sentence 2 (`δ̓`), sentence 31, sentence 36.
- Over-cap: sentences 8 (65 units), 44 (52), 52 (58), 94 (58).
- Rubric-only native segments: sentences 78, 82, 84, 90, 121 (`Νόμος.`, `Μάρτυρες.`).
- Question-mark `;`: sentences 113, 115, 116.
- Absent in this unit: homograph digits and `.R`/`.I` notation (checked, none present).

## Blockers and limits

1. **Comparator independence:** the printed Lamb 1930 edition was not checked. The comparator's licence line and header are declared, not verified.
2. **Unexplained variants:** two blocks remain (μήτε split; οὔτε/οὕτε). They need a source decision, not a fold.
3. **Convention status:** the elision/clitic/punctuation fold is a probe rule, not a reviewed convention. Exact agreement is not claimed.
4. **Vocabulary:** canonical lemma mapping, POS interpretation, and rubric exclusion remain undone. The conditional counts are diagnostics only.
5. **Sentence completeness:** not certified; native boundaries only; the 4 over-cap sentences and the rubric segments need a reviewed convention.
6. **Rights:** the AGDT file carries no licence line, so the root README declaration is the only claim. Layer review is attributed only.
7. **Dictionary:** not measured.
8. **Human review:** no example is approved.

## Self-check

- Nothing passes on arbitrary similarity. Exact token equality and span slicing are the only checks; percentages are reported as descriptive, not as gates.
- Incomplete identities are not hidden: 36 unaligned selected occurrences are counted, and the two unexplained blocks are named.
- Dry-run output makes no semantic Gloss claims. There are no glosses, no cards, and no LLM or provider calls.
- This is candidate evidence, not acceptance. The MVP decision stays with the human owner (#2030).
