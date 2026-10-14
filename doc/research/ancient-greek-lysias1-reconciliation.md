# Lysias I evidence reconciliation: full-node join and route readiness (#2048)

**Verdict: FAIL. No route is vocabulary-ready for the complete speech.** The strongest supported route is **T1A2**: pinned grc2 text with AGDT v1.6 as the primary annotation, keeping v2.1 as evidence. That annotation-release change is a proposal. It is not adopted here.

Under T1A2:

- 2,791 of 2,802 printed nodes have evidenced text correspondence.
- 1,160 of 1,395 vocabulary-candidate occurrences have resolved identities.
- 244 nodes carry at least one blocker. Of these, 235 are vocabulary candidates, 3 are non-vocabulary words with binding blockers, and 6 are punctuation marks with no grc2 counterpart.

This report joins two immutable inputs and does not repeat their investigations:

- the #2048 text-authority and binding evidence at [dc3e5ed7](https://github.com/justin-hayes/mouseion/commit/dc3e5ed74681a24157d99316c25f2e18c141f748);
- the #2049 mapping evidence at [50f4ad6c](https://github.com/justin-hayes/mouseion/commit/50f4ad6ccb5df46daf603a14dc1d1393cc274c52).

It covers every native node. No row is dropped, reclassified as non-vocabulary or relabelled resolved to make a count pass. It selects no vocabulary and measures no sentence or card coverage. Final MVP acceptance stays with #2030, and #2047 stays blocked.

Artifacts are in [`ancient-greek-lysias1-reconciliation/`](ancient-greek-lysias1-reconciliation/README.md), with rights and attribution in [`NOTICE.md`](ancient-greek-lysias1-reconciliation/NOTICE.md).

## 1. Inputs and method

**Pinned inputs.** `inputs/pinned-inputs.tsv` lists 22 roles. `inputs/sha256sums.txt` holds 61 file hashes. `scripts/fetch_inputs.sh` exports the two reconciled folders by `git archive <commit>`, never from a worktree head. The pinned raw files are:

| Input | Pin |
|---|---|
| AGDT v2.1, v2.0 and v1.6 Lysias I | `treebank_data@bf4334f0` (`44ecf3a2…`, `e3ad8b19…`, `5c8f187a…`) |
| grc2 | `canonical-greekLit@01b725d8` (`21dbae42…`) |
| grc1 | `canonical-greekLit@aa0dd9e6` (blob `418c8f2e`, sha256 `3ef8231a…`) |

**What `scripts/reconcile.py` does:**

- Joins the native nodes one-to-one. No witness or corpus is fetched.
- Re-derives the grc2 running text with the pinned binding extractor and checks it against the recorded hash, `e60cfa97…`.
- Re-verifies every claimed correspondence code point by code point.
- Evaluates four explicit reconciliation rules (XR-*).
- Assesses six routes node by node.

## 2. Full-node join (all checks pass)

**Node counts.** The 2,834 nodes are:

- 2,428 word nodes
- 374 punctuation nodes
- 32 artificial nodes (`artificial="elliptic"`)

That gives 2,802 printed nodes. Of these, 2,792 are running text, 5 are rubric label words and 5 are rubric stops (from the mapping's `source_scope`).

**Keys.** v2.1, v2.0 and v1.6 have the same `sNNNwNNN` keys, in the same order. The binding `key` set and the mapping `binding_join_key` set are both equal to the native set, with no duplicates and no gaps. Both investigations recorded the same source sha256.

**Field agreement.** Every native field each investigation copied matches the raw v2.1 file. The mapping `form` column is the NFC of the raw form.

**v1.6 compared with v2.1 at the same node id:**

- **Relation, head, artificial and POS letter:** identical on all 2,834 nodes.
- **Postag feature positions:** 137 nodes differ (positions 9, 6, 2 and 3). This is morphology, which no vocabulary rule consumes.
- **Forms:** 50 elision marks differ. 9 forms differ only by NFC, and all 9 are artificial nodes.
- **Lemmas:** 83 printed lexical lemmas lose a trailing Morpheus number, plus 1 on an artificial node. 68 lemmas are NFC-only. 374 punctuation lemmas were levelled.
- **Mixed numbering:** no lemma base mixes numbered and unnumbered occurrences. Only δέω, λέγω and ἄπειμι carry two different numbers.

**Raw versus NFC.**

- Raw v2.1 forms are NFC except for 5 elided forms: `s004w007`, `s038w012`, `s047w033`, `s052w012` and `s123w003`. In each, rho + U+0313 composes under NFC to U+1FE4, a rho with psili, which erases the separate elision mark.
- `nodes.tsv` keeps raw code points, NFC code points where they differ, and hashes.

**Binding re-verification:**

- 2,346 exact word nodes and 368 marks equal the grc2 text raw, not just after NFC.
- The 5 rubric words equal their label documents.
- In the T1 routes, all 2,764 non-component bound nodes of v1.6 are raw code-point equal to grc2.

**Input-specific issue found.** For the 22 forward component nodes, the binding manifest carries hyphen-tiled *piece* spans (for example `s009w017` 1423:1424 and `s009w018` 1424:1429). Those are component substring offsets derived from string tiling. They are reported in `b_span` but are **not adopted**: ADR 0094 says never to guess component offsets.

## 3. Routes assessed

| Route | Text authority | Annotation | Text correspondence (printed) | Vocabulary-ready occurrences | Vocabulary-ready |
|---|---|---|---:|---:|---|
| T1A1 | grc2 `01b725d8` | v2.1, input statuses as published (baseline) | 2,719 | 1,137 / 1,395 | no |
| T1A1X | grc2 | v2.1 + splice rules XR-ELISION-1, XR-DISC-1, XR-SPAN-SHARED-1 | 2,791 | 1,160 / 1,395 | no |
| **T1A2** | grc2 | **v1.6 primary** (XR-V16-PRIMARY-1, XR-SPAN-SHARED-1); v2.1 retained as evidence | 2,791 | 1,160 / 1,395 | no |
| T2A1 / T2A2 | annotation-carried forms | v2.1 / v1.6 | 0 | 0 | no (route fails) |
| T3A2 | grc1 `aa0dd9e6` | v1.6 | op-level only, no spans | 0 | not assessable |

### 3.1 Text authority

**T1 (pinned grc2): pass with qualifications.**

- It is one Perseus digitization of Lamb 1930. The canonical history from 2014 to the pin has 2 token changes.
- Full-speech collation against Mnemosyne and Wikisource confirms:
  - the bounds, with first and last words aligned;
  - 50 sections, 5 rubrics and the section 22 lacuna.
- At 55 Hude page points, grc2 agrees at 32 and differs at 23. At word level it departs from Hude at 16 places. 11 of those are documented edition alternatives. 5 have no apparatus support: `cg2:w1034`, `w2083`, `w2102`, `w2180` and `w2239`.
- Qualifications:
  - the Lamb print is UNPERFORMED and was not retried;
  - Hude was not collated in full;
  - MN and WS share the Hude tradition.
- These are dc3e5ed7's results, consumed from its committed collation files. The parent has not repeated its page readings.

**T2 (annotation-carried): fails.** AGDT carries no whitespace, paragraphing, label structure or TEI `del`/`add`/`gap` markup. The 26 component nodes are producer notation, not printed surfaces. Any running text would have to be reconstructed. The only available validation of such a reconstruction is the grc1/grc2 text itself, which makes it a composite of the two texts. ADR 0094 forbids casual joining.

T2 is not a stronger authority than T1. The annotation's own readings at `s125w015` and the rubric stops are already recorded as variant and annotation-only evidence under T1.

**T3 (grc1 at `aa0dd9e6`): not assessable at span level.**

- It is the declared URN current on the annotation date. It is an evidenced comparison baseline, **not a proven producer input revision**.
- The only evidence is dc3e5ed7's op-level comparison: v1.6 differs from it by exactly 20 operations. No span manifest exists for that blob.
- Compared with T1, it would:
  - make `s002w023` exact;
  - turn the lexical node `s060w009` (iota subscript, fixed upstream in 2016) into a variant;
  - leave `s125w015` a variant;
  - carry no licence declaration at that revision.
- It is weaker than T1 for vocabulary.

### 3.2 Named items

| Item | Nodes | T1 handling (T1A1X, T1A2) | Status |
|---|---|---|---|
| Sentence 125 breathing | `s125w015` (non-vocabulary CCONJ, R03Y) | Anchored positional binding to `cg2:w2180`. One code point differs (U+1F54 in the annotation against U+1F55 in grc2). Hude, MN and WS print the annotation reading. | **Blocking for exact text correspondence.** No factual check can make the pinned grc2 token equal; a Lamb check would only explain it. Whether to accept an explicit variant record for this one non-vocabulary node is a binding-policy question for the parent. It is not a Greek-reading choice. |
| Sentence 2 mark | `s002w023` (AuxK, the sentence-final node) | Annotation-only. The mark was present in grc1 2014 and deleted by `b01744e8` (2021). Hude and MN print none. | Explained. Not vocabulary. It leaves AGDT sentence 2 without a terminal mark in grc2, so it is a segmentation question. |
| 5 rubric stops | `s078w002`, `s082w002`, `s084w002`, `s090w002`, `s121w002` | Annotation-only. Hude prints them; grc1 and grc2 do not. The rubric words bind to label documents `cg2:label01`–`05`. | Explained. Rubric sentences are headings and stay separate from running text. |
| Iota subscript | `s060w009` | Exact against grc2. A variant against grc1 2014. | Resolved under T1. |
| Editorial `del`/`add` | `del`: `s014w007`, `s043w003`, `s113w001`; `add`: `s083w008`, `s108w020`, `s116w022` | Exact and bound. The flags are carried in `canon_flags`. | Not blocking for binding. Bracket status must reach any sentence presentation. Hude does not print `cg2:w2037` (`s116w022`). |
| Lacuna | gap at offset 6644, inside AGDT sentence 59's span | No nodes. The bounds are unchanged. | Not a binding gap. Sentence 59 contains an incomplete-text gap, which is a sentence/card question. |
| Full bounds | first and last word; 50 sections | Confirmed by three witnesses. | Resolved. |

## 4. Elision (50 nodes): XR-ELISION-1 verified on all 50

At every one of the 50 nodes, all of the following hold:

- v1.6 has a final U+02BC, and v2.0/v2.1 have the same string with that mark replaced by U+0313.
- v2.0 equals v2.1.
- The lemma and POS letter are unchanged.
- The v1.6 form equals the bound grc2 token code point for code point.

**What this is.** The v2.0 README states that "the texts have been checked/normalized for the Greek characters", and the v2.1 header names v1.6 as the text before normalization. XR-ELISION-1 is therefore the producer's own release mark mapping, proven node by node. It is not a comparison fold and not a Mouseion normalization profile. It decodes v2.x marks back to the producer's pre-normalization encoding; it does not expand elision or rewrite citation lemmas.

**Why raw forms matter.** In 5 nodes, raw v2.1 has rho + U+0313, and NFC composes that to U+1FE4. Consumers must keep the raw form; an NFC-only pipeline loses the elision mark there.

**Effect per route.**

| Route | Status of the 50 nodes |
|---|---|
| T1A2 | Exact, because the v1.6 forms are the text |
| T1A1X | Bound by the explicit rule |
| T1A1 (baseline) | Conditional, as dc3e5ed7 published them |

## 5. Components

**Forward crasis: 22 nodes, 11 grc2 tokens. XR-SPAN-SHARED-1 verified on all 22.**

For each pair:

- The preceding and following word nodes are exact bindings to the adjacent word tokens. The block therefore holds exactly these two nodes and this one token.
- The node order is fragment then host. That is the printed order and the C-CRASIS notation that 50f4ad6c attests 91 times in 11 files.
- The hyphen-stripped forms concatenate to the token in node order.

The anchors carry the proof. Concatenation is only consistent evidence. Every witness prints the unsplit word.

**Result.** Both components share the token's whole span, which ADR 0094 explicitly allows for linguistic components. No substring offset is assigned, and no claim is made about which letters belong to which component. C-CRASIS stays "attested practice, undocumented", and that affects only sub-token highlighting, which is not provided.

**Vocabulary effect.**

- 5 hosts are eligible ADJ/ADV with resolved identities: `s009w018`, `s047w026`, `s048w008`, `s053w003` and `s071w017`.
- `s118w029` (καί, ADV) is resolved.
- 4 καί/τε fragments keep their conditional eligibility.

**Sentence 11: 4 nodes, still blocking.** Exact anchors pin `s011w025`+`w026` to `cg2:w0264` and `s011w028`+`w029` to `cg2:w0266`. The question is not the pairing. The issue is that the annotation puts the `-τε` fragment *before* its host:

- that contradicts the text order;
- it also contradicts corpus practice. Outside Lysias I, 50f4ad6c counts 9 hyphen-initial enclitic fragments: 6 follow and attach to their host, 3 are classed "other", and none precedes its host.

Read by that attested convention, `s011w025` would attach to `s011w024`, whose exact token contains no τε. Applying a shared span would silently choose an interpretation that the producer's own notation contradicts. So nothing is rearranged and no span is assigned.

**Overlap.** 2 of the 4 nodes (`s011w026` and `s011w029`, μή) are also conditional particles.

**Minimum next check.** Inspect the pinned source of the tokenizer named in the v1.6 header (`services.perseids.org/llt/segtok`) for its enclitic split order. The same check may also document C-CRASIS. Neither investigation consulted this tokenizer. The check is not launched here.

## 6. Lost discriminators: v1.6 primary versus splice

**The loss.** The v2.0 normalization stripped Morpheus homograph numbers from 83 printed lexical lemmas (AGDT 2.0 guidelines: the numbers "should be deleted"). Within v2.1 that merges three pairs of entries, 15 nodes in all:

| v2.1 identity | v1.6 entries |
|---|---|
| δέω\|VERB | 1 ×1, 2 ×2 |
| λέγω\|VERB | 2 ×5, 3 ×1 |
| ἄπειμι\|VERB | 1 ×1, 2 ×5 |

The 15 nodes are `s009w016`, `s018w009`, `s021w011`, `s026w005`, `s028w008`, `s030w010`, `s046w020`, `s048w009`, `s057w004`, `s065w009`, `s071w019`, `s074w032`, `s076w020`, `s101w030` and `s111w032`.

**What the join verifies.** Node ids, POS letters, relations and heads are identical. Every printed v1.6 lexical lemma is the v2.1 lemma plus optional trailing digits after NFC. Every input to a 50f4ad6c UPOS or headword rule (letter, relation, head, artificial and decoded lemma) is therefore identical under either release, and that carry-over is asserted in code.

**Two evidenced options:**

- **T1A2 (recommended): v1.6 as the primary annotation.**
  - Each number is decoded into a `morpheus-homograph` discriminator from the *same* record as its lemma, so nothing is spliced.
  - The elision forms are already the text.
  - The 15 conflated occurrences resolve into distinct identities, and the 68 other numbered occurrences keep their discriminators.
  - Costs:
    - The 137 v2.0 postag feature corrections are absent. That matters only if a later feature consumes morphology; it would then need its own evidence.
    - The v1.6 rights question (section 9).
    - The Morpheus numbering's lexicon version is unknown. It must stay recorded as unknown, not equated with the pinned Morpheus `ab6898ff`.
- **T1A1X: keep v2.1 primary and apply XR-DISC-1**, a versioned splice that takes the number from the same node id. The outcome is identical, but the analysis must declare two field origins (ADR 0094 allows this with explicit rules and verified correspondence). The node correspondence is verified.

Neither option silently restores numbers. Both keep v2.1's stripped lemma and v1.6's numbered lemma as separate fields. Plain lemmas are recorded as `absent_in_source`, not as wildcards.

## 7. Vocabulary identity (50f4ad6c statuses, unchanged)

**Eligibility.** No conditional case is relabelled resolved. The 232 conditional-eligibility occurrences are:

| Group | Nodes | Detail |
|---|---:|---|
| Particles | 222 | 23 distinct raw lemmas. By rule: R03N 32, R03C 70, R03A 46, R03S 2, R03P 72. |
| εἰμί without PNOM | 2 | `s002w022`, `s119w006` |
| Name-like | 8 | `s004w016`, `s011w005`, `s013w009`, `s052w050`, `s083w014`, `s083w015`, `s085w012`, `s085w013` (R01C 4, R01M 2, R01A 2; 8 nodes, not the prose's 10) |

**Headwords.**

| Status | Nodes | Lemma |
|---|---|---|
| Conditional (H3) | `s072w007`, `s079w016` | ἀντί-βολέω |
| Unresolved (H4) | `s116w019` | σύν-λέγω: the v1.6 number 1 disagrees with Morpheus's only compound row, number 2 |

**Non-vocabulary conjunctions.** 14 nodes (R08X 11, R09 3) have every candidate outside `AllowedPOS`, so they need text correspondence only.

**What remains unperformed.** The full Smyth particle list (§§2769–3003), the other Morpheus noun files and the Morpheus `vbs.cmp.lsj`/`lsj.vbs` compound rows were not checked. These are unperformed checks, not proven impossibilities. A source-backed headword needs no dictionary page.

## 8. Review provenance (claims recorded, no bar invented)

- **Lemma and POS layers.** File-level human annotation by a named annotator (Vanessa Gorman, 2014). It was Morpheus-assisted (v2.0 README: "semi-automatically annotated"), and v2.0 added release-editor normalization.
- **v1.6 header.** It names the same annotator plus the segtok and Arethusa tools.
- **Stability across releases.** POS letters are identical in v1.6 and v2.1. Lexical lemmas are identical apart from the stripped numbers.
- **Not stated anywhere.** No second annotation and no per-token certification is recorded. Neither is required.
- **Syntax.** Syntax claims are not lemma/POS review.
- Whether this meets the MVP's human-reviewed bar is decided in #2030.

## 9. Rights by component and operation

Detail is in `route_ledger.json` `rights` and in NOTICE.md. Each declaration was verified in the fetched files.

| Component | Declaration | Private research | Private NC personal use | Public redistribution / adaptation |
|---|---|---|---|---|
| grc2 | CC BY-SA 4.0 (TEI licence + `license.md`) | yes | yes | eligible after #2035 exact-component review (not done) |
| AGDT v2.1 | root README CC BY-SA 3.0 US, third-party clause | yes | yes | eligible after review (not done) |
| AGDT v1.6 | same root README; the AGDT 1.7 docs README declares CC BY-NC-SA 2.5, and its reach to this schema-1.6 file is **not established** | yes, under either | permitted under either candidate declaration; no review outcome recorded | **unresolved** |
| grc1 `aa0dd9e6` | none: no licence element and no licence file in that tree | lineage facts only | unresolved | unresolved |

**Consequences for the routes:**

- T1A2 and T1A1X both depend on v1.6-derived values, so public adaptation of either needs the v1.6 question settled.
- The private personal MVP is permitted under either candidate declaration.
- Witness readings (MN, WS, Hude) are research evidence, not product data.

## 10. Readiness ledger (T1A2; all routes in `route_ledger.json`)

| Criterion | Result |
|---|---|
| Full scope and authority | Complete speech, 2,834 nodes, not shortened. T1 authority passes with the qualifications in 3.1. |
| Exact occurrences | 2,791 of 2,802 printed nodes have evidenced text correspondence: 2,764 exact, 5 label, 22 shared span. **5 word nodes are blocked**: 4 in sentence 11 and `s125w015`. 6 marks are annotation-only. |
| Required identities | 1,160 resolved, 234 conditional and 1 unresolved of 1,395 eligible-or-conditional nodes. 471 resolved eligible identities (key\|UPOS\|discriminator). **Fail.** |
| Segmentation | Native AGDT sentences only. No reviewed, versioned segmentation convention. Sentences 2, 59, the 5 rubric sentences and the del/add sentences need explicit handling. |
| Review provenance | File-level human annotation claim present; acceptance pending #2030. |
| Rights | Private use permitted under every candidate declaration. Public adaptation of v1.6-derived data unresolved. |
| Syntax | Optional; no mapping rule consumed beyond relation and head, which are identical across releases. |
| Text authority / binding / vocabulary / sentence-card | Pass with qualifications / **fail (5 nodes)** / **fail (235 nodes)** / not assessed (blocked by the earlier gates). |

## 11. Exact remaining blockers (T1A2) and overlap

| Blocker | Nodes | Layer | Nature | Minimum bounded next check (not launched) |
|---|---:|---|---|---|
| BI-ELIG-PARTICLE | 222 | identity | factual gap with unperformed checks | For each of the 23 lemmas, compare the pinned UD grc PART/CCONJ/ADV docs and the full Smyth particle list. Record a lemma-and-relation rule only where it is documented. |
| BI-ELIG-NAME | 8 | identity | gap with unperformed checks | The remaining pinned Morpheus noun stem files for the 6 lemmas |
| BI-ELIG-EIMI | 2 | identity | factual gap | Pinned UD grc cop/AUX docs on existential εἰμί |
| BI-HEADWORD-CONDITIONAL | 2 | identity | factual gap | Morpheus `vbs.cmp.lsj`/`lsj.vbs` compound rows |
| BI-HEADWORD-UNRESOLVED | 1 | identity | factual gap | Same tables. If they still disagree, the node stays unresolved. |
| BT-S011-ORDER | 4 | binding | factual gap | Pinned segtok tokenizer source |
| BT-S125-VARIANT | 1 | binding | inherent to T1 | None factual. Parent decision whether an explicit variant record may bind this one non-vocabulary node. |
| BS-PUNCT-ANNOTATION-ONLY | 6 | segmentation | explained | None for vocabulary. A reviewed segmentation convention is needed. |

**Overlap.**

- The only intersection is `s011w026` and `s011w029`, which are in both BI-ELIG-PARTICLE and BT-S011-ORDER.
- The union is 244 nodes: 235 vocabulary candidates, 3 non-vocabulary words (`s011w025`, `s011w028`, `s125w015`) and 6 marks.
- The exclusive counts are: particles 220, names 8, εἰμί 2, headword conditional 2, headword unresolved 1, s011 2, s125 1, marks 6.

**Resolved by this reconciliation.** Compared with the T1A1 baseline, these are now resolved with evidence:

- 50 elision bindings
- 22 crasis bindings, as shared spans
- 15 conflated identities

The baseline union was 315 nodes.

## 12. Not done (by constraint)

- No new witness, corpus or source survey, and no re-fetch of MN or WS.
- No Lamb/Loeb Internet Archive, Gorman PDF or OGA access, and no Scaife.
- No LLM, NLP, provider or card work, and no annotation correction.
- No vocabulary selection and no card-coverage measurement.
- No edits to #2021, #2030, #2047 or #2049.
- No schema, dependency, ADR or glossary changes.
- The T3 span manifest was not produced, so T3 stays op-level.

## 13. Reproduction and tests

`ancient-greek-lysias1-reconciliation/README.md` gives the commands.

- **Fresh run.** `fetch_inputs.sh` was run from the public GitHub URLs into a new scratch directory, and all 61 hashes verified.
- **Regeneration.** `reconcile.py` regenerated `manifest/` byte-identically.
- **Tests.** `test_reconcile.py` passed all 29 tests: 20 synthetic, plus 9 source-backed covering the fixtures, population invariants, ASCII output and byte-identical regeneration.
- **Earlier suites.** The 14 + 14 binding and collation tests were re-run from the scripts exported from dc3e5ed7, and the 23 mapping tests from the scripts exported from 50f4ad6c, against the retained #2049 repro inputs. All passed. That re-checks the inputs' reproducibility, not their interpretations.
