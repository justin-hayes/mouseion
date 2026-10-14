# Lysias I text authority and occurrence binding (#2048)

Status (revision 3): **the text-authority question is now answered for the complete speech. The binding pass condition is still not met, so #2048 stays open.**

- The full speech was collated word by word against two distinct public digital witnesses. Every point where a witness differs was then checked against page scans of a named printed edition (Hude, Oxford Classical Text).
- The canonical-greekLit history was also replayed from 2014 to the pinned revision.
- Together these settle the text's identity, its bounds, and every place where its wording differs between the canonical file, the annotation and the witnesses.
- What remains open are annotation conventions, not text authority: the 50 elision folds and the 26 component nodes (#2049's scope). Two route decisions also remain for the parent: which reading to bind at sentence 125, and which revision `s002w023` is bound against.
- The publisher's 1930 Loeb print is still not checked. Nothing is excluded from the full speech.

Section 0 is the revision 3 result. Sections 1 to 10 are the revision 2 binding probe (2cd5ed22). They are kept unchanged except where section 0 corrects or extends them, and those places are marked "r3". Corrections to 2214055f are in section 9.

Companion files: [`ancient-greek-lysias1-binding/`](ancient-greek-lysias1-binding/README.md) (scripts, manifests, tests), [evidence ledger](ancient-greek-lysias1-binding/evidence-ledger.md), [rights and provenance](ancient-greek-lysias1-binding/NOTICE.md). Revision 3 outputs are in `ancient-greek-lysias1-binding/collation/` and `fixtures/`.

## 0. Revision 3: text authority and full-speech collation

### 0.1 What the canonical text and the annotation are (lineage, verified)

`lineage_collation.py` extracts every distinct blob of the Lysias I Greek file in `canonical-greekLit` history, 18 blobs from 2014-08-18 to the pinned blob `bd5e2daf` (`collation/lineage_revisions.tsv`). It applies one fixed extraction to each blob and compares their token sequences.

- **One digital text.** grc1 was CTS-ized from the Perseus 1992 encoding of the 1930 Loeb (Lamb) in `dc36cfe1` (2014-08-18, perseids_docs#142). Its revisionDesc records "July 2014 … split composite text and converted to unicode" and "August 2014 … converted milestones to divs". The file was renamed grc1 to grc2 under #401/#403 on 2017-04-28 and 2017-05-01, and the grc1 file was deleted in `94a195bc`.
- **Only 2 token-level text changes in 12 years** (`collation/lineage_changes.tsv`):
  1. `0a3eab7f` (2016-06-17, "missing iota subscript", issue #122): section 22, token `cg2:w1090`. This is the native `s060w009` occurrence.
  2. `b01744e8` (2021-04-20, "various reported errors"): deletes the U+00B7 mark after the section 1 token `cg2:w0042`. That is exactly the mark the annotation still carries as `s002w023`.
- **Markup changes only, everywhere else.** One markup change matters: in `82af5ab7` the five rubrics moved from a `<quote><title>` at the end of sections 28, 29, 30, 31 and 42 to a `<label>` at the start of the following section. That produced the canonical label sections 29, 30, 30, 32 and 43. Rubric *section* numbers are therefore revision-specific. Use label ordinals as keys, never section numbers.
- **The annotation's base.** The AGDT header gives `annotationDate` "Thu Aug 21 … 2014" and `document_id` `urn:cts:greekLit:tlg0540.tlg001.perseus-grc1`. Its profileDesc says the "treebanked text before normalization and harmonization" is the v1.6 file.
- **AGDT v1.6 compared with grc1 at `aa0dd9e6`.** That revision was current on the annotation date. The v1.6 printed sequence differs from it by exactly 20 operations (`collation/agdt_v16_vs_annotated_base.tsv`), and nothing else:
  - 11 forward split pairs (22 nodes)
  - 2 reversed pairs in sentence 11 (4 nodes)
  - 5 rubric stops added by the annotation
  - the iota subscript at `s060w009`: the annotation has it, and grc1 gained it only in 2016
  - the breathing at `s125w015`: the annotation has smooth breathing, and grc1 had rough breathing then and still does
- **AGDT v2.1** adds the 50 elision-mark changes (U+02BC to U+0313) on top of these.

**Consequence.** AGDT and grc2 are the same digital lineage. Agreement between them is not independent corroboration, as revision 2 said. The lineage explains every AGDT/grc2 difference except the 50 folds and the 26 component nodes, which are conventions (#2049).

### 0.2 Witness leads (bounded pass: 3 leads, all for this speech only)

| Lead | Identity and pins | Lineage assessment | Use |
|---|---|---|---|
| MN: Centre for the Greek Language, *Mnemosyne* digital library, `text_id=38` | 7 HTML pages plus the contributors page, SHA-256 pinned in `fetch_witnesses.sh` (fetched 2026-10-14) | Typed by a named typing team, with named philological editors. Base edition undeclared. Not a Perseus copy: at all 16 word-level places where grc2 departs from Hude, MN prints Hude's reading. MN departs from Hude at 3 other word points, where it has the manuscript-L reading that Hude's apparatus reports (`cg2:w1110`, `cg2:w2080`, `cg2:w2131`). It also has its own typography, e.g. U+1FBD for elision. | Full-speech word and punctuation collation |
| WS: Greek Wikisource page 7431 | Revision `102844` (`action=raw`). The full revision history was inspected: 19883 (2007, anonymous), 20458, 29293 (bot replacing U+03C1 U+03C1 with U+1FE4 U+1FE5), 48054, 86278, 91877, 102844 | Source undeclared. The 2016 edit that cites the Perseus hopper only inserted section templates; its word diff is empty. Readings mix the Hude tradition with readings Hude's apparatus attributes to manuscript L or to conjecture. It has evident transcription faults: section 29 is pasted twice, the section 39 marker is numbered 29, and there are single-word slips. Weakest witness. | Full-speech collation; corroboration only |
| Hude OCT: *Lysiae Orationes*, rec. C. Hude, Oxford (Clarendon) | Internet Archive `lysiaeorationes00lysi_1`, leaves n18–n26. That copy lacks the leaf for sections 15–23, which was taken from `lysiaeorationes0000lysi` (leaves n39, n42). Image SHA-256 values are in the evidence ledger. The scanned copy's imprint names printer J. Johnson; IA metadata gives 1911. | Named critical edition, independent of the Perseus/Lamb digital line. Diacritic-critical points were checked on IIIF crops. | Printed-page check of every point where MN or WS differs from grc2, plus all named items (55 points) |

The pass did not need a fourth lead. The excluded routes in section 8 were not retried.

### 0.3 Full-speech collation results (`witness_collation.py`)

**Comparison key.** The key (K1–K5 in the script docstring) folds only these features:
- NFC
- the elision-mark code point
- case
- rho breathing inside a word (the Wikisource bot edit)
- rubric capitals

Accents, breathings, iota subscript, spelling and word division stay significant. Every raw difference the key hides is listed in `collation/witness_key_equalities.tsv`:

| Witness | Case | Elision mark | Rubric capitals | Rho breathing |
|---|---:|---:|---:|---:|
| MN | 19 | 50 | 5 | – |
| WS | 17 | 50 | 5 | 1 |

| | MN | WS |
|---|---:|---:|
| Witness word tokens | 2,420 | 2,477 |
| Canonical word tokens aligned (of 2,415 incl. 5 labels) | 2,414 | 2,415 |
| Equal word tokens | 2,400 | 2,388 |
| Non-equal word operations (all in `witness_word_ledger.tsv`) | 19 | 33 |
| …reading / diacritics only / word division / editorially marked / block insert | 13 / 4 / 1 / 1 / 0 | 22 / 9 / 1 / 0 / 1 |
| Punctuation accounted (canonical / witness) | 368 / 400 | 368 / 462 |
| Non-equal punctuation gaps (all in `witness_punct_ledger.tsv`) | 37 | 76 |
| Section markers, first–last | 1–50, sequential | 1–50, 2 out of sequence |
| First and last canonical word aligned to first and last witness word | yes / yes | yes / yes |

**Bounds and extent.** All three witnesses start and end where grc2 does, with 50 sections and the same five rubrics. MN and WS also carry the section 22 lacuna (as dots), and both carry the canonical `del`/`add` passages in brackets; only MN lacks the added word `cg2:w2037`, which Hude does not print either. No witness has additional or missing text, apart from:
- WS's duplicated section 29 paragraph, a witness fault;
- single-word variants, all listed in the word ledger.

**Cross-witness.** At 8 canonical words both MN and WS differ from grc2, and at 7 of those 8 they agree with each other. MN and WS differ from each other in 27 operations. They share the Hude tradition and are not shown to be independent of each other.

### 0.4 Printed-page point checks (`fixtures/hude_oct_page_checks.tsv` → `collation/witness_point_checks.tsv`)

I transcribed 55 points from the Hude page images. The script asserts that every word-ledger point has one (`word_ledger_points_without_page_check: []`). Agreement with Hude, as raw/key-equal against differs:

| Stream | Agrees | Differs |
|---|---:|---:|
| grc2 (pinned) | 32 | 23 |
| AGDT v2.1 | 35 | 20 |
| MN | 47 | 8 |
| WS | 20 | 35 |

- **grc2 against Hude.** grc2 departs from Hude at 16 word-level places. 11 of them are a different edition's text, not noise. Hude's apparatus attributes grc2's reading:
  - to manuscript L: `cg2:w0540`, `cg2:w1073`, `cg2:w1158`, `cg2:w1177`, `cg2:w2228`;
  - to "rell.": `cg2:w2410`, the word absent after `cg2:w0334`, and the vocative particle absent after `cg2:w1598`;
  - to the absence of a conjectural supplement: the word absent after `cg2:w1172`;
  - to a different emendation at section 41. grc2 prints Lamb's supplement `cg2:w2037` (TEI `add`) and has no article after `cg2:w2039`. Hude prints no supplement and has Bergk's article after `cg2:w2039`. Hude's apparatus gives L a different article there, which is what WS has.
- **Unexplained grc2 points.** Five grc2 points have no apparatus support:
  - three accent differences, at `cg2:w1034`, `cg2:w2102` and `cg2:w2239`
  - one word joined, at `cg2:w2083`
  - the rough breathing at `cg2:w2180`

  Whether these are Lamb's typography or Perseus encoding slips cannot be decided without the Loeb print, which is UNPERFORMED (section 8).
- **Section 7 lacuna.** Hude also prints a lacuna after section 7's last word ("lacunam statuit H. Schenkl") where grc2 has a full stop. That is an editorial difference between the editions.

### 0.5 Named items, resolved as far as text authority goes

| Item | Native keys | Finding | Status |
|---|---|---|---|
| Sentence 125 breathing | `s125w015` ↔ `cg2:w2180` | AGDT has smooth breathing (U+1F54), and so do Hude (checked on a crop), MN and WS. grc1 had rough breathing (U+1F55) from 2014 and grc2 still does. The canonical token is the outlier; the annotation form agrees with every external witness. | **Text side resolved** as a canonical-file variant. The route decision (bind the annotation-carried reading or keep a recorded variant against grc2) is the parent's. Lamb's print is unverified. |
| Sentence 2 mark | `s002w023` (after `cg2:w0042`) | Present in grc1 at the annotation date. Deleted from grc2 by `b01744e8` as a "reported error". Hude and MN print no mark; WS has a comma. | **Explained**: a lineage revision difference. Against pinned grc2 it is correctly unaligned. Against grc1 at `aa0dd9e6` it would be exact. |
| Sentence 11 | `s011w025`+`w026` ↔ `cg2:w0264`; `s011w028`+`w029` ↔ `cg2:w0266` | Every witness prints the single unsplit word at both places: grc1 2014, grc2, MN, WS, and Hude (the section 6 line). Text and anchors are settled. | Text resolved. Component order is an annotation convention (#2049). |
| 11 forward concatenations | 22 nodes | The canonical crasis tokens equal MN and WS raw at all 11. grc1 2014 already had them unsplit. | Text resolved. The split convention belongs to #2049. |
| 50 folds | 50 nodes | The elided words are present in every witness (key-equal). The mark is encoded differently in each: U+1FBD in MN, U+0027 in WS, U+02BC in grc2 and AGDT v1.6, U+0313 in AGDT v2.x. The AGDT header calls v1.6 the text "before normalization and harmonization". | The text is resolved, but the elision code point is not something the witnesses can settle. The v2.x normalization convention belongs to #2049. |
| Iota subscript | `s060w009` ↔ `cg2:w1090` | The annotation already had the subscript in v1.6. grc1 lacked it until `0a3eab7f` (2016). Hude, MN and WS have it. | Resolved; exact against grc2. |
| 5 rubrics and 5 stops | `s078w001`…`s121w001`; stops `s078w002`…`s121w002` | All witnesses have the 5 rubrics in the same positions. Hude prints each rubric word in capitals followed by a full stop, which matches the AGDT AuxK nodes. grc1, grc2, MN and WS have no stop. | Rubric words resolved. The stops are attested in Hude's printing but absent from the annotation's declared base. They are annotation-carried, not repaired. |
| Lacuna | after `cg2:w1075` | grc1 and grc2 use `<gap/>`. Hude prints dots with Reiske's supplement in the apparatus; MN and WS print dots. The AGDT nodes have nothing printed there. | Resolved; bounds unchanged. |

### 0.6 Route consequences (for the parent; no selection made)

- **Canonical-backed binding against pinned grc2 (revision 2 route).**
  - Text identity is now explained: one Perseus digitization of Lamb, with full-speech bounds confirmed by three witnesses.
  - At `s125w015` the pinned token disagrees with every external witness and with the annotation. Binding there needs an explicit variant record, not a repair.
  - `s002w023` stays unaligned against grc2, by design of the 2021 correction.
- **Annotation-carried text (ADR 0093 permits this when verified).**
  - The AGDT surfaces equal grc1 2014 except at the 20 operations in 0.1, and each of those is accounted for: 13 are conventions, 5 are rubric stops attested in print, and 2 are corrections matching all external witnesses.
  - Its printed text still carries the 50 U+0313 elision forms and the hyphenated components. Those need #2049's convention findings before spans over a reconstructed text can be claimed.
- **grc1 at `aa0dd9e6` (the declared `perseus-grc1` URN at annotation time).**
  - It differs from pinned grc2 by exactly 2 tokens.
  - Binding to it would make `s002w023` exact and `s060w009` a one-code-point mismatch.
  - The rights of the historical blob were not reviewed (NOTICE.md).
  - This keeps the declared identity without relabelling grc1 as grc2.
- **What the parent still has to join.** #2049's conventions for the 50 folds and the 26 component nodes, and a decision on which reading each route binds at sentence 125.

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
- **r3:** the lineage is now replayed from 2014 to the pin (0.1). The full speech has been collated against two distinct public witnesses with a printed-edition point check (0.2–0.4). The Loeb print itself remains UNPERFORMED.

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
- **r3:** every text witness prints the unsplit canonical word at both positions (0.5). The open fact is only the annotation's component-order convention, which is #2049's scope.

### 4.6 Sentence 125: one-breathing variant, candidate

- `s125w015` is the only free node and `cg2:w2180` the only free token of their block (block 60), after the exact and fold steps. The surfaces differ at one code point: U+1F54 in AGDT against U+1F55 in the canonical text (a breathing mark only; the length is equal).
- The AGDT form is identical in v2.0, v1.6 and v2.1. The probe records it as a positional candidate (`surface = mismatch`, `candidate`). It is not repaired. Which text is correct cannot be decided from the pinned inputs.
- **r3:** decided as far as external witnesses go.
  - The annotation's smooth breathing is printed by Hude (checked on a crop), MN and WS.
  - The canonical rough breathing goes back to grc1 in 2014, so it is an outlier in the canonical lineage.
  - The probe status stays `candidate`. Choosing which reading a route binds is the parent's call (0.6). Nothing is repaired.

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
- **r3:**
  - The section numbers above are grc2's. grc1, at the annotation date, placed the same five rubrics at the end of sections 28, 29, 30, 31 and 42, as Hude prints them. Keys use label ordinals for this reason.
  - Hude prints each rubric with a full stop. The AGDT stops are attested in print, though not in the annotation's base text.

### 4.8 Punctuation

- 368 canonical running punctuation tokens and 368 AGDT printed punctuation nodes match exactly inside gaps between word anchors. The links are recorded in both directions.
- `s002w023` (sentence 2, a printed mark) has no canonical counterpart in its gap. It is `unaligned` and is not repaired.
- **r3:** explained. grc1 had this mark when the text was annotated. grc2 deleted it in `b01744e8` (2021-04-20, "various reported errors"). Hude and MN print no mark; WS has a comma.

### 4.9 Artificial nodes (32)

Recorded in `occurrence_binding.tsv` with `occurrence_status = not_printed`. They are not bound.

## 5. Tests and reproduction

- `scripts/test_binding_probe.py`: 14 focused tests on synthetic input only, covering repeated rubric labels, unmatched rubric nodes, mixed-tag TEI (`q`, `del`, `add`, `note`, `gap`, labels containing `q`), NFD and NFC span integrity, elision-fold equivalence, fold candidate versus conditional, exact runs, ambiguous identical components, reversed-only concatenation, and a concatenation that spans an exact anchor. Result: 14 OK.
- Reproducer from a fresh `fetch_inputs.sh` (new scratch directory): both hashes OK; the regenerated manifest is byte-identical to the committed `manifest/` (`diff -r` empty).
- Committed manifests are ASCII only (checked by grep). The running-text hash matches the value recorded above.
- **r3:**
  - `scripts/test_witness_collation.py` has 14 synthetic tests and they pass. They cover elision-mark attachment, quotes, the rho fold scope, significant accents, the disjoint rubric key, Wikisource and Mnemosyne extraction, lineage extraction, and the word-division/diacritics/block classes and comparator.
  - Fresh reproduction ran in a new directory: `fetch_inputs.sh` then `fetch_witnesses.sh`, with all pinned hashes OK. `binding_probe.py` regenerated `manifest/` byte-identically. `lineage_collation.py` and `witness_collation.py` regenerated `collation/` byte-identically.
  - The collation outputs are ASCII. Surfaces appear only as U+XXXX sequences.

## 6. Route consequences for #2030 (no selection made)

- The full Lysias I unit is the requirement. Sentence 11, sentence 125 and the sentence 2 mark are part of that unit. None is excluded here, and card eligibility (#2047) cannot waive text or occurrence readiness.
- Canonical-backed binding (this report's route) gives 2,346 accepted and 50 conditional running tokens, and 14 candidate-only running tokens. Its text authority is single-lineage (section 4.1).
- No downstream slice should be treated as text-ready until the candidate, conditional and unresolved items in section 7 are settled.
- **r3:** see 0.6. The single-lineage limitation is now characterised, and the full speech is collated against external witnesses. The remaining route inputs are #2049's conventions and the parent's sentence 125 / `s002w023` binding choice.

## 7. Unresolved and UNPERFORMED facts

Revision 3 list. It supersedes the revision 2 list, which is kept below for history.

**UNPERFORMED** (not run, and why):

1. **The grc2 header's print source (Lamb, Loeb 1930).**
   - The Internet Archive fetch was interrupted by the user and has not been retried by any route.
   - Scaife is a copy of the same line and returned a proof-of-work challenge, which was not bypassed.
   - Consequence: it is undecided whether the five grc2 points with no apparatus support (0.4) are Lamb's print or encoding slips. That includes the sentence 125 breathing.
2. **A full word-by-word collation of the Hude scan.**
   - Only the 55 points in the fixture were transcribed from the page images.
   - Hude's OCR text exists, but it was not used because of its noise.
   - The full-speech collation was done against MN and WS.
3. **The Gorman PDF and the OGA archive** were not accessed, by constraint.

**Unresolved or outside this track** (with the evidence needed):

4. **Annotation conventions (#2049):**
   - the sentence 11 component order;
   - the split convention for 11 forward pairs;
   - the v2.x U+0313 elision normalization for 50 nodes.

   The text side of all three is resolved (0.5).
5. **Sentence 125 binding choice.** The text evidence is complete. A route decision is needed (0.6).
6. **`s002w023` binding.** It is explained, but it stays unaligned against pinned grc2 unless the route binds to grc1 at `aa0dd9e6` (0.6).
7. **MN's base edition is undeclared.** MN and WS are not shown to be independent of each other (0.3).

Revision 2 list (superseded):

1. Independent witness for the full unit: UNPERFORMED. Revision 3 performed it with MN, WS and Hude points.
2. Sentence 11 component order (4.5).
3. Sentence 125 breathing variant (4.6).
4. Sentence 2 mark `s002w023` (4.8).
5. 50 fold nodes (4.3) and 22 concatenation nodes (4.4).

## 8. Access log

| Source | Declared role | Access | Status |
|---|---|---|---|
| `treebank_data` @ `bf4334f0` (v2.1, v2.0, v1.6 files and READMEs) | Native annotation | fetched by git, hash verified | inspected |
| `canonical-greekLit` @ `01b725d8` (grc2 file and `license.md`) | Canonical text | fetched by git, hash verified | inspected |
| Loeb 1930 scan via Internet Archive (named in grc2 header) | Candidate independent witness | webfetch attempt interrupted by user; not retried | UNPERFORMED |
| Scaife reader (`scaife.perseus.org`, grc2 line) | Hosted copy of the same line | anti-bot proof-of-work challenge; not bypassed | access restricted, UNPERFORMED |
| Gorman PDF; OGA archive | Excluded by ticket constraints | not accessed | not performed |
| r3: `canonical-greekLit` history (18 blobs, 2014-08-18 → pin) | Lineage of the canonical text | git, blob ids asserted | inspected, full |
| r3: Centre for the Greek Language, Mnemosyne `text_id=38` (7 pages, contributors page) | Distinct digital witness | public HTTPS, robots.txt allows `/digitalResources/`, hashes pinned | inspected, full-speech collation |
| r3: Greek Wikisource page 7431, revisions 19883–102844 | Distinct digital witness (weak) | MediaWiki `action=raw` and revisions API | inspected, full-speech collation and history |
| r3: Hude OCT scans, IA `lysiaeorationes00lysi_1` (n18–n26, title leaves via IIIF) and `lysiaeorationes0000lysi` (n39, n42) | Named printed edition | IA page images, IIIF crops, djvu.txt/djvu.xml/scandata for page location | inspected at 55 points; the first copy lacks the section 15–23 leaf, which was taken from the second copy |

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
- Committed artifacts contain hashes, code-point offsets, ASCII keys, classes and counts. Attribution is in NOTICE.md.
- **r3:**
  - `collation/` and `fixtures/` record the readings at differing points as U+XXXX sequences, from grc2, AGDT, MN, WS and Hude. These are exact Greek readings in escaped form. ASCII encoding does not by itself settle derivative-rights questions.
  - Committed witness content is limited to the differing tokens and punctuation:
    - Wikisource's duplicated section 29 paragraph appears in full, as code points, in one row. Wikisource text is CC BY-SA 4.0.
    - MN pages state "© 2012 Kentro Ellinikis Glossas, all rights reserved" (transliterated). Their text is not committed; it is re-fetched by URL and hash. Only the individual readings at differing points are recorded.
    - Hude's text is recorded only at the 55 transcribed points. The edition is Oxford/Clarendon, and IA metadata dates it 1911, which makes it a pre-1929 publication. Its rights were not reviewed in this pass.
  - See NOTICE.md.
