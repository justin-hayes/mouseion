# Evidence ledger (#2048)

Status values: **verified** (reproduced from pinned inputs by script), **conditional** (binding holds on the pinned texts but depends on an undocumented convention or a single lineage), **unresolved** (not bound; blocker named).

| ID | Claim | Evidence | Reproduce | Status |
|---|---|---|---|---|
| E01 | AGDT v2.1 file matches pinned SHA-256 | `sha256sum -c` in `fetch_inputs.sh` | `fetch_inputs.sh` | verified |
| E02 | grc2 file matches pinned SHA-256 | `sha256sum -c` in `fetch_inputs.sh` | `fetch_inputs.sh` | verified |
| E03 | Canonical running text: 50 sections, 15,110 code points, NFC, 5 labels, 1 lacuna (offset 6,644) | `manifest/summary.json` → `running_text` | `binding_probe.py` | verified |
| E04 | Denominators: 2,834 nodes = 2,428 word + 374 punct + 32 artificial | `manifest/summary.json` → `denominators`; `occurrence_binding.tsv` (2,834 rows) | `binding_probe.py` | verified |
| E05 | 2,346 word nodes bound by exact NFC equality | `summary.json` → `word_node_classes.exact_1to1`; `occurrence_binding.tsv` `binding_class` | `binding_probe.py` | verified |
| E06 | 368 punct nodes exact match canonical punctuation | `summary.json` → `punct_node_classes.punct_exact_punct` | `binding_probe.py` | verified |
| E07 | 2,407 of 2,410 canonical word tokens bound; `cg2:w0264`, `cg2:w0266`, `cg2:w2180` unbound | `manifest/canonical_tokens.tsv` (2,410 rows) | `binding_probe.py` | verified |
| E08 | Canonical punctuation (368) matched 368/368 to AGDT punctuation | probe `punct_alignment_ops` (equal 10 blocks) | `binding_probe.py` | verified (counted only; **not keyed per row**) |
| E09 | 50 fold nodes bind by elision-fold (45) or NFD fold (5); AGDT carries U+0313, canonical U+02BC | `fold_mechanisms.json`; `occurrence_binding.tsv` | `fold_and_release_checks.py` | conditional (exact surface differs in v2.0/v2.1) |
| E10 | The 50 fold nodes match canonical exactly in AGDT v1.6 (U+02BC) | `release_stability.tsv`; `summary` line `agdt-v1.6 ... diff 50` | `fold_and_release_checks.py` | verified (release witness) |
| E11 | AGDT v2.0 equals v2.1 on all 2,802 printed nodes | `release_stability.tsv`; script output `agdt-v2.0 same 2802 diff 0` | `fold_and_release_checks.py` | verified |
| E12 | 22 concatenation nodes (11 trailing-hyphen, 11 none) exact in code points, forming 6 canonical tokens | `fold_mechanisms.json`; `occurrence_binding.tsv` | `binding_probe.py` | conditional (no documented split convention found) |
| E13 | Concatenation nodes identical in v1.6, v2.0, v2.1 | `release_stability.tsv` | `fold_and_release_checks.py` | verified |
| E14 | 5 rubric-label nodes bind exactly to canonical `<label>` (sections 32, 43, 32, 32, 43) | `discrepancies.tsv` rows `s078w001 … s121w001` | `binding_probe.py` | verified (not running-text) |
| E15 | 5 rubric-attached AuxK punctuation have no running-text counterpart | `summary.json` → `punct_node_classes` | `binding_probe.py` | verified (classification) |
| E16 | Sentence 11 forward `-τε`+`μή` gives `τεμή` (no match); reversed gives `μήτε` | `discrepancies.tsv` `reversed_order_candidate_not_bound` (4 rows) | `binding_probe.py` | unresolved |
| E17 | Sentence 11 pairing ambiguous: each reversed window matches both `μήτε` canonical tokens | `summary.json` → `reversed_order_candidates` | `binding_probe.py` | unresolved |
| E18 | No AGDT guideline convention for enclitic/elision/crasis order found | AGDT2 markdown 2.0 and 2.5 (none); GSAAGDT 1.1 PDF text (partial extraction) | manual search of `docs/` text, not in committed artifacts | conditional (search incomplete for GSAAGDT) |
| E19 | Sentence 125 `s125w015` `οὔτε` vs canonical `cg2:w2180` `οὕτε` | `discrepancies.tsv` (`s125w015`); `canonical_tokens.tsv` | `binding_probe.py` | unresolved |
| E20 | Sentence 125 `οὔτε` form identical in v1.6, v2.0, v2.1 | `release_stability.tsv` | `fold_and_release_checks.py` | verified (stability only) |
| E21 | `εἴη·` (sentence 2) AuxK has no canonical counterpart | `summary.json` → `punct_node_classes.punct_punct_unaligned` (1) | `binding_probe.py` | unresolved |
| E22 | AGDT header states text derives from canonical-greekLit; text lineage is single | AGDT header (read in earlier step) | manual | verified (declaration) |
| E23 | No permitted independent witness verified for full-unit text | Loeb 1930 fetch interrupted and not retried; Scaife/Perseus = grc2 line; hypotactic.com = AGDT-style placeholder | manual | unresolved |
| E24 | Artificial nodes (32) recorded, not bound | `occurrence_binding.tsv` `node_class=artificial` | `binding_probe.py` | verified |
| E25 | Full reproduction from clean scratch matches committed manifests byte-for-byte | `diff -r` of regenerated `manifest/` vs committed | README steps | verified |
| E26 | Greek surfaces absent from committed manifests (ASCII only) | `grep -P '[^\x00-\x7F]'` on `manifest/*` returns 0 | shell check | verified |
