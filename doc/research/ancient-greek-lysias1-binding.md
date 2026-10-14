# Lysias I text authority and occurrence binding (#2048)

Status: **bounded investigation incomplete; #2048 stays open.** Full-unit text authority is not established. The only independent-witness routes were access-restricted (section 8), so that check is UNPERFORMED, not negative. Sentence 11, sentence 125 and one sentence 2 punctuation mark remain unresolved. Nothing is excluded from the full speech.

This revision supersedes the 2214055f report. Corrections to that version are in section 9.

Companion files: [`ancient-greek-lysias1-binding/`](ancient-greek-lysias1-binding/README.md) (scripts, manifests, tests), [evidence ledger](ancient-greek-lysias1-binding/evidence-ledger.md), [rights and provenance](ancient-greek-lysias1-binding/NOTICE.md).

## 1. Result

| Population | Total | Accepted | Conditional | Candidate | Ambiguous | Unbound |
|---|---:|---:|---:|---:|---:|---:|
| AGDT word nodes (printed) | 2,428 | 2,351 (2,346 exact + 5 rubric) | 50 (fold) | 27 (22 concat + 4 reversed + 1 positional) | 0 | 0 |
| AGDT punctuation nodes (printed) | 374 | 368 (exact, gap-local) | 0 | 0 | 0 | 1 unaligned, plus 5 annotation-only (rubric stops) |
| AGDT artificial nodes | 32 | not printed | | | | |
| Canonical running word tokens | 2,410 | 2,346 | 50 | 14 | 0 | 0 |
| Canonical label word tokens | 5 | 5 | 0 | 0 | 0 | 0 |
| Canonical running punctuation tokens | 368 | 368 | 0 | 0 | 0 | 0 |

Denominators: 2,834 AGDT nodes = 2,428 word + 374 punctuation + 32 artificial. The statuses are per-node and per-token and are kept separate (section 3.2). "Accepted" means exact NFC equality on a monotone equal run, or a rubric match whose anchor agrees. "Conditional" is a fold match with a release-witness equality and no documented convention. "Candidate" is a proposed correspondence that is not accepted.

Every canonical punctuation token is linked to exactly one AGDT punctuation node, and the reverse link is also recorded (`occurrence_binding.tsv` `canon_keys`, `canonical_tokens.tsv` `node_keys`).

## 2. Inputs

| Role | Repository @ revision | Path | SHA-256 |
|---|---|---|---|
| Native annotation (primary) | `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` | `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` | `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568` |
| Canonical running text | `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c` | `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` | `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a` |
| Release witness (stability only) | `treebank_data` @ same rev | `v2.0/Greek/nonArethusaCompliant/tlg0540.tlg001.perseus-grc1.tb.xml` | `e3ad8b19166c6dffcc0e0b8a3d634b262fba0ef27a4b31329fa838ffaa78e467` |
| Release witness (stability only) | `treebank_data` @ same rev | `v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml` | `5c8f187a489c67fa06bb70219f27177c8dd9b8061e198b9ee63a56f3d9bd9e27` |

The two primary hashes are checked by `scripts/fetch_inputs.sh`. The witness hashes are printed by the same script and are not asserted by the probe. The inputs are not committed.

## 3. Method

### 3.1 Canonical text and tokens

- Running text: the `<p>` text of the 50 edition sections in document order. `<note>` apparatus is excluded. `<label>` text is excluded from the running text and recorded as its own document with a TEI address. `<del>`, `<add>`, `<q>` and `<foreign>` text is kept and flagged. `<gap reason="lost">` adds no characters and records an offset.
- Result: 15,110 code points, NFC, 56 paragraph elements in the sections, 5 labels, one lacuna at offset 6,644. Running-text SHA-256 `e60cfa97121a64d86f1a5c495ee3484cb63ddd730abe5749f9ca387485d29815`.
- Tokens: a word is a maximal run of Unicode L* or M* characters. Every other non-space character is a punctuation token. Labels are tokenised as separate documents (`cg2:labelNN`, keys `cg2:LNNwNNN`).

### 3.2 Alignment and statuses

- **Global monotone alignment, then local blocks.** Printed AGDT word nodes (rubric nodes excluded) are aligned to running tokens once, globally, with `difflib` on NFC forms. This is *not* a per-sentence alignment; the 2214055f description of per-sentence alignment was wrong. Equal runs are exact and accepted. Each unequal run between equal runs is a **local block** (63 blocks; largest 2 nodes and 2 tokens).
- **Fold and concatenation are searched only inside one block.** Nothing is matched across an exact anchor. Fold: elision-like final marks unified to U+02BC on NFD forms. Concatenation: windows of 2 or 3 consecutive free nodes inside the block whose hyphen-stripped pieces tile the token exactly in code points. A window is a *candidate* correspondence only, unless a documented convention supports it (none was found).
- **Ambiguity.** If one token has more than one forward window in its block, or a forward and a reversed window, nothing is bound and every involved node is `ambiguous`. No such case occurs in the pinned text.
- **Rubric.** A rubric node is a single-word sentence whose form equals a label text. Rubric nodes are bound to label documents by **sequence position** (difflib on the rubric sequence against the label sequence), so repeated strings bind to distinct labels. Each binding is checked against the neighbouring running anchors on both sides.
- **Punctuation.** Punctuation is matched only inside the gap between two consecutive word anchors (accepted or conditional), so distant equal marks cannot pair.

Status fields are kept separate in `occurrence_binding.tsv`:

- `correspondence`: exact_equal, exact_within_block, fold_equal, concat_window, concat_reversed, positional_block, rubric_sequence, punct_gap, none.
- `surface`: what the surfaces show (exact, fold_elision, fold_nfd, concat_exact, label_exact, mismatch, none).
- `convention`: not_applicable, none_documented, or cross_release_v1.6_equal. The last is a release-witness equality. It is **not** a documented convention.
- `occurrence_status`: accepted, conditional, candidate, ambiguous, unbound, annotation_only, not_printed.

Canonical coverage (`canonical_tokens.tsv` `coverage_status`) is accepted only if some linked node is accepted. It is conditional if only conditional nodes link to it, candidate_only if only candidates do, and ambiguous or unbound otherwise.

### 3.3 Keys

Rows are keyed by native AGDT sentence and word IDs (`sNNNwNNN`, v2.1). Canonical tokens are keyed `cg2:wNNNN` (running), `cg2:pNNNN` (running punctuation), `cg2:LNNwNNN` and `cg2:LNNpNNN` (label documents). Every span is a half-open code-point range into the document named in its row, and every row records that document's SHA-256. Spans are never used as keys.

## 4. Findings

### 4.1 Full-unit text authority: lineage declared, independent check UNPERFORMED

- The AGDT v2.1 header says the digital edition "can be consulted on PerseusDL (canonical-greekLit), looking up the cts:urn", and that "the annotator(s) may have chosen different variants for the text". AGDT and grc2 therefore share a declared lineage. Their agreement is not independent corroboration.
- The grc2 header cites a print source (William Heinemann Ltd. and Harvard University Press, with an Internet Archive scan of the 1930 Loeb). That print text is the only candidate independent witness named in the pinned inputs. It was not verified (section 8).
- The English translation in the pinned repository is not a Greek witness and was not used.

### 4.2 Exact nodes (2,346): accepted by monotone position, not by uniqueness

- All 2,346 exact nodes lie on monotone equal runs (`monotone_violations = 0`). 1,658 of them have a form that repeats elsewhere in the node or canonical sequence. They are accepted by their position in an order-preserving run, not because the form is locally unique.
- 3 exact nodes form single-node runs: `s011w027` and `s011w030` (sentence 11 anchors, 4.5) and `s048w009`. They separate blocks. The sentence 11 anchor `s011w024` sits in a 34-node run.

### 4.3 Fold nodes (50): conditional

- All 50 are elision-like folds. AGDT carries final U+0313 (`final_U+0313`, all 50). The canonical token carries U+02BC.
- AGDT v2.0 equals v2.1 on all 2,802 printed nodes. AGDT v1.6 differs from v2.1 on exactly these 50 nodes, and there it equals the canonical token.
- Status is **conditional**. The only convention evidence is the v1.6 witness equality. No documented convention was found, so the fold is not accepted.

### 4.4 Concatenation nodes (22 nodes, 11 canonical tokens): candidate

- Each of the 11 canonical tokens is tiled by exactly one forward 2-node window in its block: one node with a trailing hyphen and one without.
- No documented split convention was found in the consulted guideline text (AGDT2 markdown 2.0 and 2.5; GSAAGDT 1.1 partially). These are candidates, not accepted.
- Correction: the 2214055f report said 6 canonical tokens. The correct figure is 11.

### 4.5 Sentence 11: reversed components, candidate

Sentence 11 contains four nodes that concatenate to two canonical tokens only in reversed order (`s011w025`+`w026` and `s011w028`+`w029`, both to `cg2:w0264` and `cg2:w0266` respectively). The forward concatenation does not match.

- **What pins the pairing.** Exact anchors: `s011w024` to `cg2:w0263`, `s011w027` to `cg2:w0265`, and `s011w030` to `cg2:w0267`. Order is monotone, so the first pair lies between `w0263` and `w0265` and can only correspond to `w0264`. The second pair lies between `w0265` and `w0267` and can only correspond to `w0266`. Identical spelling at two positions is therefore not by itself ambiguous, because the anchors fix the positions.
- **What is missing.** The component order. The forward order does not match and the reversed order does. The only leading-hyphen forms in the pinned annotation are these two pairs. No within-file or consulted-guideline convention for that order was found. The search is limited to the consulted guideline text, so the convention is unresolved rather than shown absent.
- **Status.** Both pairs are candidates and are linked to their canonical tokens. Neither is accepted. No order is inferred from string coincidence, and no reordering or substring is fabricated.

### 4.6 Sentence 125: one-breathing variant, candidate

- `s125w015` is the only free node and `cg2:w2180` the only free token of their block (block 60), after the exact and fold steps. The surfaces differ at one code point: U+1F54 in AGDT against U+1F55 in the canonical text (a breathing mark only; the length is equal).
- The AGDT form is identical in v2.0, v1.6 and v2.1. The probe records it as a positional candidate (`surface = mismatch`, `candidate`). It is not repaired. Which text is correct cannot be decided from the pinned inputs.

### 4.7 Rubric labels (5 nodes, 5 labels): accepted by sequence and anchor

| Node | Label document | Section | TEI address | Anchor agrees (prev / next) |
|---|---|---:|---|---|
| `s078w001` | `cg2:label01` | 29 | `div[@n='29']/p[1]/label[1]` | yes / yes |
| `s082w001` | `cg2:label02` | 30 | `div[@n='30']/p[1]/label[1]` | yes / yes |
| `s084w001` | `cg2:label03` | 30 | `div[@n='30']/p[3]/label[1]` | yes / yes |
| `s090w001` | `cg2:label04` | 32 | `div[@n='32']/p[1]/label[1]` | yes / yes |
| `s121w001` | `cg2:label05` | 43 | `div[@n='43']/p[1]/label[1]` | yes / yes |

- The label texts repeat (three with one string, two with another). Each occurrence binds to its own label by sequence position. The 2214055f dictionary collapsed repeated strings and assigned sections 32 and 43 to the duplicates; that was wrong (section 9).
- The five AuxK stops (`s078w002`, `s082w002`, `s084w002`, `s090w002`, `s121w002`) are `annotation_only`. The canonical label text carries no stop mark.

### 4.8 Punctuation

- 368 canonical running punctuation tokens and 368 AGDT printed punctuation nodes match exactly inside gaps between word anchors. The links are recorded in both directions.
- `s002w023` (sentence 2, a printed mark) has no canonical counterpart in its gap. It is `unaligned` and is not repaired.

### 4.9 Artificial nodes (32)

Recorded in `occurrence_binding.tsv` with `occurrence_status = not_printed`. They are not bound.

## 5. Tests and reproduction

- `scripts/test_binding_probe.py`: 14 focused tests on synthetic input only, covering repeated rubric labels, unmatched rubric nodes, mixed-tag TEI (`q`, `del`, `add`, `note`, `gap`, labels containing `q`), NFD and NFC span integrity, elision-fold equivalence, fold candidate versus conditional, exact runs, ambiguous identical components, reversed-only concatenation, and a concatenation that spans an exact anchor. Result: 14 OK.
- Reproducer from a fresh `fetch_inputs.sh` (new scratch directory): both hashes OK; the regenerated manifest is byte-identical to the committed `manifest/` (`diff -r` empty).
- Committed manifests are ASCII only (checked by grep). The running-text hash matches the value recorded above.

## 6. Route consequences for #2030 (no selection made)

- The full Lysias I unit is the requirement. Sentence 11, sentence 125 and the sentence 2 mark are part of that unit. None is excluded here, and card eligibility (#2047) cannot waive text or occurrence readiness.
- Canonical-backed binding (this report's route) gives 2,346 accepted and 50 conditional running tokens, and 14 candidate-only running tokens. Its text authority is single-lineage (section 4.1).
- No downstream slice should be treated as text-ready until the candidate, conditional and unresolved items in section 7 are settled.

## 7. Unresolved and UNPERFORMED facts

**UNPERFORMED** (not run, and why):

1. Independent witness for the full unit. The Loeb 1930 scan named in the grc2 header was attempted with a webfetch, which the user interrupted. It was not retried or reached by another route. The Scaife reader was tried and returned an anti-bot proof-of-work challenge. That is an access restriction, so the attempt was not repeated or bypassed. The Gorman PDF and OGA archive were not accessed under the ticket constraints. No other permitted public witness was consulted in this pass.

**Unresolved** (with the sources consulted):

2. Sentence 11 component order (4.5). Needed: a documented convention or an attested-occurrence source.
3. Sentence 125 breathing variant (4.6). Needed: an attested-occurrence source or an independent witness.
4. Sentence 2 mark `s002w023` (4.8). Its canonical counterpart is not located.
5. 50 fold nodes (4.3) remain conditional, and 22 concatenation nodes (4.4) remain candidates, pending a convention or witness.

## 8. Access log

| Source | Declared role | Access | Status |
|---|---|---|---|
| `treebank_data` @ `bf4334f0` (v2.1, v2.0, v1.6 files and READMEs) | Native annotation | fetched by git, hash verified | inspected |
| `canonical-greekLit` @ `01b725d8` (grc2 file and `license.md`) | Canonical text | fetched by git, hash verified | inspected |
| Loeb 1930 scan via Internet Archive (named in grc2 header) | Candidate independent witness | webfetch attempt interrupted by user; not retried | UNPERFORMED |
| Scaife reader (`scaife.perseus.org`, grc2 line) | Hosted copy of the same line | anti-bot proof-of-work challenge; not bypassed | access restricted, UNPERFORMED |
| Gorman PDF; OGA archive | Excluded by ticket constraints | not accessed | not performed |

## 9. Corrections to the 2214055f version

- The `verified` flag is removed. Statuses are separated into correspondence, surface, convention and occurrence status (3.2).
- Alignment: the previous report said per-sentence `difflib`. The probe aligns globally and restricts fold and concatenation to local blocks.
- Rubric: the previous collapsed dictionary assigned sections 32 and 43 to duplicate labels. The correct sections are 29, 30, 30, 32 and 43 (4.7).
- Concatenation: 11 canonical tokens, not 6 (4.4).
- Canonical coverage: the previous "2,407 of 2,410 bound" counted candidate and conditional tokens as bound. It is now 2,346 accepted, 50 conditional and 14 candidate-only.
- The recommendation to exclude sentences 11 and 125 from a first slice is withdrawn. The full unit is required.
- "No permitted independent witness" is replaced by UNPERFORMED (section 7), because one interrupted fetch and one access restriction do not establish that none exists.
- The 2214055f report and ledger contained Greek surfaces in their text. This version contains none. The earlier commit and the earlier issue comment still contain them. History was not rewritten, and the parent should decide on that.
- `fold_and_release_checks.py` was removed. Its release comparison is now part of the probe. `fold_mechanisms.json` was replaced by the `fold_and_concat_mechanisms` block of `summary.json`.

## 10. Rights and provenance (summary; detail in NOTICE.md)

- Native annotation: the `treebank_data` root README declares contents CC BY-SA 3.0 US, and excludes material to which third parties hold copyright. The v2.1 header and README carry no licence element. The v1.6 tree's README declares none.
- Genuine uncertainty: the v1 Greek docs README (AGDT 1.7 documentation) declares CC BY-NC-SA 2.5. Whether that declaration governs the v1.6 data file is not established. The committed `release_stability.tsv`, the `witness_v16_equal_canonical` column and the conditional status all depend on v1.6 data.
- Canonical text: the grc2 file's TEI header and the repository `license.md` both declare CC BY-SA 4.0.
- Committed artifacts contain hashes, code-point offsets, ASCII keys, classes and counts. They contain no Greek text. Attribution is in NOTICE.md.
