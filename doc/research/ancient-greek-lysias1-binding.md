# Lysias I text authority and occurrence binding (#2048)

Status: **conditional. The pass condition is not met.** A reproducible binding route exists from the pinned AGDT v2.1 annotation to a canonical-greekLit grc2 running text. Full-unit text authority is not independently established, because both pinned texts share one lineage. Sentence 11 and sentence 125 remain unresolved, and so does one sentence 2 punctuation mark.

Companion files: [`ancient-greek-lysias1-binding/`](ancient-greek-lysias1-binding/README.md) (scripts, manifests, README), [evidence ledger](ancient-greek-lysias1-binding/evidence-ledger.md), [rights notices](ancient-greek-lysias1-binding/NOTICE.md).

## 1. Pinned inputs

| Role | Repository @ revision | Path | SHA-256 |
|---|---|---|---|
| Native annotation (primary) | `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` | `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` | `44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568` |
| Canonical running text (comparator) | `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c` | `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` | `21dbae42979097132d8d4c16531ed0527beac717b2cc1fffec541205f6514d6a` |
| Release witness (node-ID stability only) | `treebank_data` @ same rev | `v2.0/Greek/nonArethusaCompliant/tlg0540.tlg001.perseus-grc1.tb.xml` | `e3ad8b19166c6dffcc0e0b8a3d634b262fba0ef27a4b31329fa838ffaa78e467` |
| Release witness (node-ID stability only) | `treebank_data` @ same rev | `v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml` | `5c8f187a489c67fa06bb70219f27177c8dd9b8061e198b9ee63a56f3d9bd9e27` |

The hashes were checked by `scripts/fetch_inputs.sh` from fresh clones of both repositories. The inputs are not committed. Re-fetch instructions are in the README.

Only AGDT v2.1 and grc2 are used as text sources. The v2.0 and v1.6 files are used only to test whether the same native node IDs carry the same forms.

## 2. Method

1. **Canonical running text.** Take the `<p>` text of each of the 50 edition sections in document order. `<note>` apparatus is excluded, and `<label>` rubric text is kept as element addresses, not running text. `<del>`, `<add>` and `<q>` text is kept and flagged. `<gap reason="lost">` adds no characters. The result is 15,110 code points, NFC, 5 labels, one lacuna at offset 6,644. Spans are half-open code-point offsets. The running-text SHA-256 is `e60cfa97121a64d86f1a5c495ee3484cb63ddd730abe5749f9ca387485d29815`.
2. **Tokens.** Word tokens are maximal runs of Unicode L* or M* characters. Every other non-space character is a punctuation token. This gives 2,410 canonical word tokens and 368 canonical punctuation tokens.
3. **Node alignment.** AGDT nodes are aligned to the canonical text per sentence with `difflib`. Binding applies three rules in order:
   1. Exact NFC equality within equal blocks.
   2. Elision-fold equality inside unequal blocks, where the final elision-like mark is unified to U+02BC and both sides are NFD.
   3. Exact code-point concatenation of 2 or 3 consecutive nodes whose hyphen-stripped forms concatenate to one canonical token.

   Reversed-order concatenation is recorded as a candidate only and is never bound.
4. **Keys.** Every row is keyed by the native AGDT sentence and node ID (`sNNNwNNN`) under the pinned v2.1 revision. Canonical tokens are keyed `cg2:wNNNN` under the pinned grc2 revision. Spans alone are never used as keys.

## 3. Accounting (every printed occurrence and every artificial node)

| Population | Total | Bound verified | Bound conditional | Unbound / unresolved |
|---|---:|---:|---:|---:|
| AGDT word nodes (printed) | 2,428 | 2,346 exact | 50 fold + 22 concat + 5 rubric-label | 5 |
| AGDT punctuation nodes (printed) | 374 | 368 exact | 5 rubric-attached (no running-text counterpart) | 1 unaligned |
| AGDT artificial nodes | 32 | n/a (not printed) | n/a | recorded, not bound |
| Canonical word tokens | 2,410 | 2,407 | — | 3 |
| Canonical punctuation tokens | 368 | 368 | — | 0 |

Denominators: 2,834 nodes = 2,428 word + 374 punctuation + 32 artificial. The 2,802 printed nodes are the 2,428 word and 374 punctuation nodes. The accounting reconciles: 2,423 word nodes bound, 5 unbound; 368 + 5 + 1 = 374 punctuation nodes.

`occurrence_binding.tsv` has one row for each of the 2,834 nodes (2,428 word, 374 punctuation, 32 artificial). `canonical_tokens.tsv` has one row for each of the 2,410 canonical word tokens. **Canonical punctuation (368 tokens) is counted in the probe and matched 368/368 to AGDT punctuation, but it is not keyed row by row in the manifests.** That is a gap for downstream joins, not a finding.

## 4. Findings

### 4.1 Full-unit text authority: not independently established

- AGDT's header states that its text derives from canonical-greekLit. AGDT and grc2 therefore share one lineage, and their agreement is not independent corroboration.
- The Loeb 1930 page was attempted with a webfetch and the attempt was interrupted at the user's direction. It was not completed and is not used.
- Scaife/Perseus copies of the same grc2 line are not independent. hypotactic.com shows an AGDT-style `[0]` placeholder, not an independent text.
- Result: the binding is reproducible to the pinned texts, but completeness of the speech against an independent witness is unverified. Matching endpoints, 50 labels, and stripped-text similarity were not treated as proof.

### 4.2 Fold bindings (50 word nodes): conditional

- The 50 nodes are elision/breathing-fold matches. The AGDT form carries final U+0313 (combining comma above). The canonical form carries U+02BC.
- AGDT v2.0 and v2.1 agree on every printed node. AGDT v1.6 differs from v2.1 on exactly these 50 nodes, and there v1.6 carries U+02BC, matching canonical exactly.
- The fold is therefore supported by a release witness, but the exact surfaces differ in v2.0/v2.1. Binding them is conditional on the fold being an encoding change rather than a text difference. This is not an exact-surface match.

### 4.3 Concatenation bindings (22 nodes, 6 canonical tokens): exact text, convention not documented

- Each concatenation is exact in code points, and identical across all three releases.
- Eleven nodes carry a trailing hyphen and eleven carry none. The hyphen is stripped only for the concatenation test.
- No documented AGDT split convention was found in the Greek guidelines consulted (AGDT2 markdown 2.0 and 2.5; GSAAGDT 1.1, extracted partially). The concatenation is treated as exact, with its convention marked as undocumented.

### 4.4 Sentence 11: `-τε μή` versus canonical `μήτε`: unresolved

- Forward order gives `τεμή`, which does not match. Reversed order (`μή` + `τε`) gives `μήτε` exactly.
- Two `μήτε` canonical tokens (`cg2:w0264`, `cg2:w0266`) and two AGDT pairs (`s011w025+w026`, `s011w028+w029`) are involved. Each reversed-order window matches both canonical tokens, so pairing by order is ambiguous.
- No guideline convention was found. Per the ticket, nothing is inferred and nothing is reordered.
- Affected rows: `s011w025`, `s011w026`, `s011w028`, `s011w029` (unbound); `cg2:w0264`, `cg2:w0266` (unbound canonical). The four candidates are logged in `discrepancies.tsv` as `reversed_order_candidate_not_bound`.

### 4.5 Sentence 125: `οὔτε` versus `οὕτε`: unresolved

- AGDT node `s125w015` carries `οὔτε`. The canonical text has only `οὕτε` (`cg2:w2180`). The AGDT form is the same in all three releases.
- The variant is not repaired. Which text is wrong cannot be decided here. An attested-occurrence source is needed.

### 4.6 Rubric labels (5 nodes): bound to the canonical `<label>` element

- `s078w001`, `s082w001`, `s084w001`, `s090w001`, `s121w001` are bound exactly to canonical `<label>` elements in sections 32, 43, 32, 32 and 43.
- They are not bound to running text, and they are not dropped to improve coverage.
- The 5 rubric-attached AuxK punctuation nodes likewise have no running-text counterpart and are recorded as such.

### 4.7 Sentence 2 punctuation: unaligned

- `εἴη·` (AuxK, sentence 2) has no counterpart: the canonical text has no mark there. It is recorded as `punct_unaligned` and not repaired.

### 4.8 Sentence and card eligibility

Not computed here (per the ticket). The sentences containing unresolved items are 11, 125 and the sentence 2 punctuation. Any slice that includes them is conditional. For the live #2030 decision, the route consequences are:

- **Route A: canonical-backed binding (this report's route).** Covers 2,407/2,410 canonical word tokens and 2,423/2,428 AGDT word nodes. The running text is grc2, which AGDT declares it derives from. Independence is not established.
- **Route B: annotation-carried authority (AGDT v2.1 as running text).** Makes the same forms available with the same node keys. It inherits the same single-lineage limitation, and it carries the sentence 125 and sentence 11 variants as annotation identities, not as resolved text.

Recommendation for the human decision: use Route A for binding, exclude sentences 11 and 125 from the first slice, and treat the text as single-lineage until an independent witness is supplied. This is a recommendation, not a selection.

## 5. Blockers

1. **Independent witness for full-text authority.** No permitted independent witness was verified. Needed: a printed critical edition or equivalent independent text the user is willing to supply or approve, checked at the sentence 11 and 125 positions and the fold/concatenation rows.
2. **Sentence 11 pairing convention.** Needed: a documented AGDT or canonical convention for `-τε μή` versus `μήτε`, or attested occurrences that settle the pairing.
3. **Sentence 125 `οὔτε`/`οὕτε`.** Needed: an attested-occurrence source.

None of these was obtainable within the permitted sources. The Gorman PDF and OGA archive were not accessed, and their access was not retried.

## 6. Rights and provenance (summary; detail in NOTICE.md)

- `treebank_data` root README declares its contents CC BY-SA 3.0 US, and states that third-party-owned material is excluded from Perseus's copyright. Layer applicability (annotation vs text) is not separately resolved here.
- `canonical-greekLit` `license.md` is CC BY-SA 4.0. Its TEI file-level licence was read in an earlier step and was not re-verified in the final run.
- Committed artifacts contain hashes, code-point offsets, ASCII keys and counts only. They contain no Greek surfaces.
- Whether the research artifacts' licence should match the repository's own licence is not determined here and needs a human decision.

## 7. Reproduction and checks

```sh
sh doc/research/ancient-greek-lysias1-binding/scripts/fetch_inputs.sh <empty-dir>/inputs
python3 -I doc/research/ancient-greek-lysias1-binding/scripts/binding_probe.py <empty-dir>/inputs <empty-dir>/manifest <empty-dir>/detail
python3 -I doc/research/ancient-greek-lysias1-binding/scripts/fold_and_release_checks.py <empty-dir>/inputs <empty-dir>/manifest <empty-dir>/detail
diff -r <empty-dir>/manifest doc/research/ancient-greek-lysias1-binding/manifest
```

From a fresh state, the hash checks pass and the regenerated manifests are byte-identical to the committed ones. The scripts have no separate unit-test suite; the check is the reproducer plus the hash verification above.

## 8. Lysias I unit

The complete speech is 50 sections, 2,834 AGDT nodes and 2,410 canonical word tokens. No other unit is covered by this report.
