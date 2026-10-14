# Evidence ledger (#2048)

Status values: **verified** (reproduced from the pinned inputs by script or test), **conditional** (holds on the pinned texts but depends on a release witness with no documented convention), **candidate** (proposed correspondence, not accepted), **unresolved** (not settled by the consulted sources; the gap is named), **UNPERFORMED** (the check was not run; the reason is named).

Manifest fields refer to `manifest/occurrence_binding.tsv` (OB), `manifest/canonical_tokens.tsv` (CT), `manifest/rubric_bindings.tsv` (RB), `manifest/release_stability.tsv` (RS), `manifest/discrepancies.tsv` (DS), `manifest/sentences.tsv` (SE) and `manifest/summary.json` (SJ).

| ID | Claim | Evidence | Reproduce | Status |
|---|---|---|---|---|
| E01 | AGDT v2.1 file matches pinned SHA-256 | `sha256sum -c` in `fetch_inputs.sh`; also asserted by probe | `fetch_inputs.sh`, `binding_probe.py` | verified |
| E02 | grc2 file matches pinned SHA-256 | as E01 | as E01 | verified |
| E03 | Witness hashes v2.0 `e3ad8b19…e467`, v1.6 `5c8f187a…9e27` | printed by `fetch_inputs.sh` | `fetch_inputs.sh` | verified (printed; not asserted by probe) |
| E04 | Running text: 15,110 code points, NFC, 50 sections, 5 labels, lacuna at 6,644, SHA-256 `e60cfa97…d29815` | SJ `running_text` | `binding_probe.py` | verified |
| E05 | Denominators: 2,834 nodes = 2,428 word + 374 punctuation + 32 artificial; 2,802 printed | SJ `denominators`; OB (2,834 data rows) | `binding_probe.py` | verified |
| E06 | Word-node statuses: 2,351 accepted, 50 conditional, 27 candidate, 0 ambiguous, 0 unbound | SJ `word_node_status`; OB `occurrence_status` | `binding_probe.py` | verified |
| E07 | Running word tokens: 2,346 accepted, 50 conditional, 14 candidate-only, 0 unbound; label tokens 5 accepted | SJ `canonical_word_coverage`; CT `coverage_status` | `binding_probe.py` | verified |
| E08 | Running punctuation: 368 accepted; links recorded in both directions | SJ `canonical_punct_coverage`; OB `canon_keys`; CT `node_keys` | `binding_probe.py` | verified |
| E09 | Punctuation nodes: 368 accepted, 5 annotation-only (rubric stops), 1 unaligned (`s002w023`) | SJ `punct_node_status`; DS | `binding_probe.py` | verified |
| E10 | All 2,346 exact nodes lie on monotone equal runs; zero monotone violations | SJ `monotone_violations = 0`, `anchor_sequence_equal = true` | `binding_probe.py` | verified |
| E11 | 1,658 exact nodes have a repeated form; they are accepted by run position, not uniqueness | SJ `exact_audit`; OB `node_form_count`, `canon_form_count` | `binding_probe.py` | verified (audit) |
| E12 | 3 exact nodes are single-node runs: `s011w027`, `s011w030`, `s048w009` | OB `equal_run_len = 1` | `binding_probe.py` | verified |
| E13 | Fold nodes (50): AGDT carries final U+0313, canonical carries U+02BC | SJ `fold_and_concat_mechanisms` (`final_U+0313`, 50) | `binding_probe.py` | verified (mechanism) |
| E14 | Fold nodes (50) bind as conditional; the only convention evidence is v1.6 equality | OB `occurrence_status = conditional`, `convention = cross_release_v1.6_equal`, `witness_v16_equal_canonical` | `binding_probe.py` | conditional |
| E15 | v1.6 equals the canonical token on all 50 fold nodes | OB `witness_v16_equal_canonical`; RS `equal_v21` | `binding_probe.py` | verified (witness) |
| E16 | v2.0 equals v2.1 on all 2,802 printed nodes | RS (`agdt-v2.0`, `equal_v21`) | `binding_probe.py` | verified |
| E17 | Concatenation (22 nodes) forms 11 canonical tokens; each token has one trailing-hyphen and one bare node in a 2-node window | SJ `concat_audit`; OB `correspondence = concat_window`; AGDT forms checked from pinned file | `binding_probe.py` | verified (structure) |
| E18 | No documented split convention for the 11 concatenations | consulted guideline text (AGDT2 markdown 2.0 and 2.5; GSAAGDT 1.1 partially) | manual search, not in the manifest | unresolved (search limited to consulted text) |
| E19 | Concatenation nodes are candidates | OB `occurrence_status = candidate`, `convention = none_documented` | `binding_probe.py` | candidate |
| E20 | No concatenation window crosses an exact anchor | test `test_concatenation_never_spans_an_exact_anchor`; SJ `blocks` (max 2 nodes) | `test_binding_probe.py` | verified (test) |
| E21 | Sentence 11 anchors: `s011w024`↔`cg2:w0263`, `s011w027`↔`cg2:w0265`, `s011w030`↔`cg2:w0267` (exact) | OB rows | `binding_probe.py` | verified |
| E22 | Sentence 11 reversed pairs (`s011w025`+`w026`, `s011w028`+`w029`) match their tokens only in reversed order; forward does not match | OB `correspondence = concat_reversed`, `surface = mismatch`, `status = candidate` | `binding_probe.py` | verified (structure) |
| E23 | Sentence 11 pairs are order-pinned to `cg2:w0264` and `cg2:w0266` by the exact anchors | derived from E21 and monotone order | `binding_probe.py` | verified (by order) |
| E24 | The component-order convention for sentence 11 is not documented in consulted sources; the only leading-hyphen forms in the pinned annotation are these two pairs | consulted guideline text; pinned AGDT scan | manual search | unresolved |
| E25 | Sentence 125: `s125w015` and `cg2:w2180` differ at one code point, U+1F54 against U+1F55 (breathing); equal length | OB `correspondence = positional_block`, `status = candidate`; probe comparison of code points | `binding_probe.py` | verified (difference) |
| E26 | Sentence 125 AGDT form is identical in v2.0, v1.6 and v2.1 | RS | `binding_probe.py` | verified (stability only) |
| E27 | Which text is correct at sentence 125 cannot be decided from the pinned inputs | — | — | unresolved |
| E28 | Rubric: 5 rubric nodes bind by sequence position to 5 label documents at sections 29, 30, 30, 32, 43; neighbours agree on both sides for all 5 | RB (`status = accepted`, `prev_agree`, `next_agree`) | `binding_probe.py` | verified |
| E29 | Repeated rubric labels bind to distinct labels by position | test `test_repeated_labels_bind_by_sequence_position` | `test_binding_probe.py` | verified (test) |
| E30 | The 5 AuxK rubric stops (`s078w002`, `s082w002`, `s084w002`, `s090w002`, `s121w002`) have no canonical counterpart | DS `annotation_only`; label text carries no stop | `binding_probe.py` | verified (classification) |
| E31 | `s002w023` (sentence 2) has no canonical counterpart in its gap | DS `unaligned` | `binding_probe.py` | unresolved |
| E32 | Label TEI addresses and mixed-tag parsing (`q`, `del`, `add`, `note`, `gap`) | tests `test_labels_get_distinct_tei_addresses_and_child_tags`, `test_running_text_excludes_labels_and_notes_keeps_flagged_text` | `test_binding_probe.py` | verified (test) |
| E33 | NFD token spans slice back to token text; NFC piece spans are exact | tests `test_token_spans_slice_back_to_token_text_for_nfd_input`, `test_piece_spans_use_nfc_code_points_of_the_token` | `test_binding_probe.py` | verified (test) |
| E34 | Fold candidate versus conditional separated by witness equality; no fold is accepted | tests `test_fold_without_witness_is_candidate`, `test_fold_with_equal_v16_witness_is_conditional_not_accepted` | `test_binding_probe.py` | verified (test) |
| E35 | Identical components matching two windows are ambiguous and unbound; reversed-only is candidate, not accepted | tests `test_identical_components_matching_two_windows_are_ambiguous_and_unbound`, `test_reversed_only_concatenation_is_candidate_not_accepted` | `test_binding_probe.py` | verified (test) |
| E36 | AGDT v2.1 header lineage statement: the digital edition "can be consulted on PerseusDL (canonical-greekLit)"; "the annotator(s) may have chosen different variants" | AGDT header, read in this step | `sed -n '1,/</header>/p'` on the pinned file | verified (declaration) |
| E37 | grc2 header cites a print source (William Heinemann Ltd.; Harvard University Press; Internet Archive scan) | grc2 `sourceDesc` | `sed` on the pinned file | verified (declaration) |
| E38 | Loeb 1930 scan (named in E37) was not verified as a witness | webfetch interrupted by user; not retried | — | UNPERFORMED |
| E39 | Scaife reader (grc2 line) gave an anti-bot proof-of-work challenge; no witness obtained | webfetch response; not bypassed | — | UNPERFORMED (access restricted) |
| E40 | Gorman PDF and OGA archive: not accessed | ticket constraints | — | not performed by design |
| E41 | Licence declarations: root README CC BY-SA 3.0 US; grc2 TEI header and `license.md` CC BY-SA 4.0; v1 Greek docs README CC BY-NC-SA 2.5 (AGDT 1.7 docs); v2.1, v2.0 and v1.6 READMEs and headers have no licence statement | greps on the pinned tree and headers, this step | `git show` / `grep -i licen` on the pinned revisions | verified (declarations); applicability to v1.6 data unresolved (NOTICE.md) |
| E42 | Reproduction from fresh `fetch_inputs.sh`: hashes OK; committed manifest byte-identical; 14 tests OK | run in this step | README steps | verified |
| E43 | Committed manifests contain no Greek characters | `grep -P '[^\x00-\x7F]' manifest/*` returns nothing | shell check | verified |
| E44 | 2214055f report and ledger contained Greek surfaces; this version contains none | pre-revision grep found 90 Greek code points in the report and ledger; re-grep of this version returns none | shell check | verified |
| E45 | The 2214055f dictionary collapse assigned sections 32 and 43 to duplicate labels; correct sections are 29, 30, 30, 32, 43 | grc2 section structure; RB | `binding_probe.py` | verified (correction) |
| E46 | Independent full-unit text witness | none obtained (E38, E39) | — | UNPERFORMED |

## Revision 3 (text authority and full-speech collation)

Supersessions: E27 (sentence 125 undecidable) is superseded by E64 on the text side. E31 (`s002w023` unresolved) is superseded by E65. E46 (no independent witness obtained) is superseded by E53–E63. The Loeb print (E38) stays UNPERFORMED as E72.

Files: `collation/` (CO), `fixtures/hude_oct_page_checks.tsv` (FX). Witness pins are in `scripts/fetch_witnesses.sh` and are asserted by `witness_collation.py`.

| ID | Claim | Evidence | Reproduce | Status |
|---|---|---|---|---|
| E47 | The 18 distinct historical blobs of the Lysias I Greek file (`dc36cfe1` 2014-08-18 … `70427323` = pinned blob `bd5e2daf`) have their git blob ids asserted | CO `lineage_revisions.tsv` | `fetch_witnesses.sh`, `lineage_collation.py` | verified |
| E48 | Only 2 token-level text changes from 2014 to the pin: iota subscript at `cg2:w1090` (`0a3eab7f`, 2016-06-17, issue #122), and deletion of U+00B7 after `cg2:w0042` (`b01744e8`, 2021-04-20) | CO `lineage_changes.tsv` | `lineage_collation.py` | verified |
| E49 | Rubric placement moved from end-of-section `quote/title` (sections 28, 29, 30, 31, 42) to start-of-section `label` (29, 30, 30, 32, 43) in `82af5ab7` | CO `lineage_revisions.tsv` `rubric_sections`, `rubric_placement_changed` | `lineage_collation.py` | verified |
| E50 | AGDT header: `annotationDate` Thu Aug 21 2014; `document_id` `urn:cts:greekLit:tlg0540.tlg001.perseus-grc1`; profileDesc names v1.6 as the "treebanked text before normalization and harmonization" | pinned AGDT v2.1 header | `sed` on the pinned file | verified (declaration) |
| E51 | AGDT v1.6 printed sequence compared with grc1 at `aa0dd9e6` (current on the annotation date): exactly 20 non-equal operations (11 forward pairs, 2 reversed pairs, 5 rubric stops, `s060w009`, `s125w015`) | CO `agdt_v16_vs_annotated_base.tsv`, `lineage_summary.json` | `lineage_collation.py` | verified |
| E52 | grc1 at the annotation date already carried the canonical rough-breathing form at `cg2:w2180`. The annotation's smooth-breathing form is not inherited from its base text | CO `agdt_v16_vs_annotated_base.tsv` (last row) | `lineage_collation.py` | verified |
| E53 | Mnemosyne pages 1–7 and contributors page pinned (SHA-256 in `fetch_witnesses.sh`). The site names a typing team and philological editors. No source edition is declared on the pages consulted | page hashes; contributors page | `fetch_witnesses.sh` | verified (declaration and absence on consulted pages) |
| E54 | Wikisource page 7431 revision history: anonymous 2007 creation; 2008 bot `-rr +rh-rh`; the 2016 edit citing the Perseus hopper only inserts section templates (empty word diff); 2017 template removals | revision hashes in `fetch_witnesses.sh`; word diffs run in this pass | `fetch_witnesses.sh` + `diff` | verified |
| E55 | Full-speech word collation of grc2 (2,415 words incl. labels) against MN (2,420) and WS (2,477): 19 and 33 non-equal operations, all listed. First and last words aligned. 50 section markers each | CO `witness_word_ledger.tsv`, `witness_summary.json` | `witness_collation.py` | verified |
| E56 | Every punctuation token accounted for: 368 canonical; 400 MN; 462 WS. Non-equal gaps are listed (37 MN, 76 WS) | CO `witness_punct_ledger.tsv`, `punct_accounted` (asserted) | `witness_collation.py` | verified |
| E57 | Raw differences hidden by the comparison key are listed by class: case 19/17, elision mark 50/50, rubric capitals 5/5, rho breathing 0/1 (MN/WS) | CO `witness_key_equalities.tsv` | `witness_collation.py` | verified |
| E58 | MN and WS differ from grc2 at 8 shared canonical words, and agree with each other at 7 of them. They differ from each other in 27 operations | CO `witness_summary.json` `cross_witness` | `witness_collation.py` | verified (not shown independent of each other) |
| E59 | Hude OCT identity: title page "LYSIAE ORATIONES recognovit … CAROLUS HUDE … OXONII E TYPOGRAPHEO CLARENDONIANO"; verso imprint "Excudebat Joannes Johnson"; IA metadata date 1911 | IA `lysiaeorationes00lysi_1` leaves via IIIF `$7`, `$8`; metadata API | manual view | verified (identity); printing date of the scanned copy not established |
| E60 | Hude page images used (SHA-256): `lysiaeorationes00lysi_1` n18 `903c3970666b4e87a950e062c4d285500258138b6f08c2cb67edfc816d2b11c3`, n19 `2ba64d2d5d65a80fc1c55bce0417bfabb7f7d351e7cd07c0c57550420548a612`, n20 `9c8cced80fe4fc4f5fc9c1ca4664ad2eda0f52be05d62952f1632196dde8d59b`, n21 `c4758cf2e8068edf148552516cc13f14cda9dc17ed5fa44a0c9160354f33123d`, n22 `1f11bdb78eaa4307cf0aa0617a0aeff0fcef1668021670cdb9145c748e1987d3`, n23 `7a2dfa5f75164ae12148bb00f6221e935ceb41b78f9f68e7545dca673a5e99b2`, n24 `6aeed2a705a5f12c1dd32f78cc132c7a455aaa0aeeab48bb63cb8e665cd63f79`, n25 `3653e91c5a07db6e7256beedef4dc54dd3f67342bce082d73399f15518d4ab7d`, n26 `fcef309e4537afacd8d7e1108fbccaa6fdd585b4dcb1f1d0152cb6886d4ae2e9`; `lysiaeorationes0000lysi` n39 `d920a00b2e68aa63461547c28d663a55055d06cc60aefd496499d97e60ecd5b8`, n42 `014376606beaef55b1567826920417b39c80e913ad00ca116e93c1da8d065444` | image downloads in this pass | `https://archive.org/download/<item>/page/n<leaf>.jpg` | verified (hashes as fetched) |
| E61 | The first Hude copy lacks the leaf for sections 15–23 (djvu objects 20→21 jump from section 15 to section 23). The second copy supplies it | `djvu.xml` object word lists; images | manual + script in this pass | verified |
| E62 | 55 Hude points transcribed. Every word-ledger point has a page check | FX; CO `witness_point_checks.tsv`; `word_ledger_points_without_page_check: []` | `witness_collation.py` | verified (manual transcription; script join) |
| E63 | Agreement with Hude at 55 points: grc2 32, AGDT 35, MN 47, WS 20 | CO `witness_summary.json` `hude_point_checks` | `witness_collation.py` | verified |
| E64 | Sentence 125: Hude (IIIF crop), MN and WS have the smooth breathing, equal to AGDT `s125w015`. grc2 `cg2:w2180` differs | CO `witness_point_checks.tsv` row `cg2:w2180` | `witness_collation.py` | verified (text side) |
| E65 | `s002w023`: grc1 2014 has the mark; grc2 dropped it in `b01744e8`; Hude and MN print none; WS prints a comma | E48; CO point row `cg2:w0042` | both scripts | verified (explained) |
| E66 | Sentence 11 and the 11 crasis tokens are printed unsplit by every witness. Component conventions are left to #2049 | CO named checks and point rows `cg2:w0264`, `cg2:w0266` | `witness_collation.py` | verified (text side) |
| E67 | 50 elided words are present in all witnesses. The elision code point is digitization-specific (MN U+1FBD, WS U+0027, grc2/v1.6 U+02BC, v2.x U+0313) | CO `witness_key_equalities.tsv` (elision_mark 50/50); named checks `fold_conditional` 50 equal_key | `witness_collation.py` | verified (text side); convention unresolved (#2049) |
| E68 | Hude prints each of the 5 rubrics with a full stop, matching the AGDT AuxK stops. grc1, grc2, MN and WS have no stop | CO point rows `cg2:L0Nw001 punct_after` | `witness_collation.py` | verified |
| E69 | 11 of grc2's 16 word-level departures from Hude are attributed in Hude's apparatus to L, rell., an absent supplement, or a different emendation. 5 have no apparatus support (`cg2:w1034`, `w2083`, `w2102`, `w2180`, `w2239`) | FX notes; page images | manual | verified (apparatus reading); Lamb's print UNPERFORMED |
| E70 | Hude marks a lacuna after section 7's last word (H. Schenkl); grc2 has none | point row `cg2:w0341` | `witness_collation.py` | verified (edition difference) |
| E71 | Fresh reproduction: all pinned hashes OK; `manifest/` and `collation/` byte-identical; 14 + 14 tests OK | run in this pass | README | verified |
| E72 | Loeb 1930 print (grc2 sourceDesc) | not fetched; interrupted route not retried | — | UNPERFORMED |
| E73 | Full Hude collation beyond the 55 points | OCR not used; images only at the points | — | UNPERFORMED |
