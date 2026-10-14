# Lysias I reconciliation artifacts (#2048)

Companion folder of [`../ancient-greek-lysias1-reconciliation.md`](../ancient-greek-lysias1-reconciliation.md). Rights and attribution: [`NOTICE.md`](NOTICE.md).

## Layout

| Path | Content |
|---|---|
| `inputs/pinned-inputs.tsv` | Registry of every input: role, repository, immutable commit, path, local path, sha256, declared rights, use. |
| `inputs/sha256sums.txt` | sha256 of all 61 files written by `fetch_inputs.sh` (including every file exported from the two reconciled commits). |
| `scripts/fetch_inputs.sh` | Fetches the inputs at pinned commits (`git archive`/`git show`, never a worktree head) and verifies them. |
| `scripts/reconcile.py` | Join, re-verification, XR rules, six route assessments, ledger. Deterministic; ASCII output. |
| `scripts/test_reconcile.py` | 20 synthetic unit tests, plus 9 source-backed tests with `RECON_INPUTS` (fixtures, invariants, byte-identical regeneration). |
| `fixtures/expected-route-nodes.tsv` | 64 hand-checked route rows for 16 named nodes, each with the source fact that decides it. |
| `manifest/nodes.tsv` | 2,834 rows, one per native node: raw and NFC evidence, both investigations' fields, re-verification results, evidence links. |
| `manifest/route_nodes.tsv` | 6 × 2,834 rows: per-route text status, identity status, readiness and blockers. |
| `manifest/blockers.tsv` | Per route and blocker: counts (all / lexical / vocabulary candidates) and every node key. |
| `manifest/canonical_coverage.tsv` | 2,783 grc2 tokens (2,410 running words, 5 label words, 368 marks) with input status and T1A1X/T1A2 status. |
| `manifest/sentences.tsv` | 134 native sentences: segmentation-state facts (final node and its T1 status, lacuna, editorial flags, blocked counts). Not card coverage. |
| `manifest/route_ledger.json` | Machine-readable readiness ledger: join checks, verification counts, rules, text authority, rights by component and operation, review provenance, per-route criteria, blocker definitions, overlap accounting, verdict. |

## Keys

- `node_key` = `sNNNwNNN`, the native AGDT sentence and word ids. They are identical, in the same order, in v2.1, v2.0 and v1.6 at `treebank_data@bf4334f0` (checked), and equal to the binding manifest `key` and the mapping manifest `binding_join_key`. They are scoped to that pinned file family, not global.
- Canonical tokens keep the binding manifest's keys: `cg2:wNNNN`, `cg2:pNNNN`, `cg2:LNNwNNN`. Spans are half-open code-point offsets into the document named in `b_canon_doc` (`cg2:run` sha256 `e60cfa97…`, re-derived and checked by the script). Spans are never keys.

## `nodes.tsv` columns

- Native: `form_v21_cp` (raw code points), `form_v21_is_nfc`, `form_v21_nfc_cp` (only when NFC differs), `form_v21_sha256`, `form_v16_rel` (`equal | elision_mark | nfc_equivalent | other`), `form_v16_cp` (only when different), `form_v20_equal_v21`, `lemma_v21_cp`, `lemma_v16_rel` (`equal | morpheus_number | nfc_equivalent | punct_levelled | morpheus_number_unprinted`), `lemma_v16_cp`, `postag_v21`, `postag_v16_diff_positions`, `relation`, `head`, `artificial`.
- Binding input (`b_*`, dc3e5ed7, unchanged): correspondence, surface, convention, status, canonical keys, document, span, block, reason. For the 22 forward components the input span is a hyphen-tiled piece span; it is **reported, not adopted** (section 5 of the report).
- Re-verification: `canon_flags` (TEI `del`/`add`/`q`), `canon_cp` (grc2 token code points, only on non-exact rows), `v16_text_rel`, `anchor_verified`, `xr_elision`, `xr_span`.
- Evidence links: `collation_refs` (`WLnnn` word ledger row, `PLnnn` punctuation ledger row, `HUDE:<point>`, `NC:<named check>`), `lineage_ref` (`LBnn` row of `agdt_v16_vs_annotated_base.tsv`).
- Mapping input (`m_*`, 50f4ad6c, unchanged): rule, UPOS candidates and status, eligibility, headword status/rule/code points, canonical key code points, discriminator status/value, identity status and candidates, component convention, evidence ids, flags.

## `route_nodes.tsv` statuses

- `text_status`: `exact`, `exact_nfc_equivalent`, `label_exact`, `bound_xr_elision`, `shared_span_anchor` (these five satisfy text correspondence); `conditional_fold`, `candidate_concat` (baseline only); `blocked_component_order`, `variant_positional`, `annotation_only_print_attested`, `annotation_only_deleted_in_grc2`, `own_surface_unspaced`, `producer_notation_not_printable`, `nonstandard_elision_encoding`, `op_level_*`, `not_printed`.
- `identity_status`: the 50f4ad6c propagation (resolved only when UPOS, headword, eligibility and discriminator are all resolved), with the route's discriminator source.
- `readiness`: `vocab_ready`, `vocab_blocked`, `non_vocab_ok`, `non_vocab_blocked`, `punct_ok`, `punct_blocked`, `unprinted`. `vocab_ready` is a per-occurrence state, not a vocabulary selection.

## Reproduce

```sh
D=$HOME/.cache/scratch/lysias1-reconciliation-repro      # new or empty directory
sh scripts/fetch_inputs.sh "$D"                          # prints "all pinned inputs verified"
python3 -I scripts/reconcile.py "$D" "$D/manifest-out"
diff -r manifest "$D/manifest-out"                       # empty
RECON_INPUTS="$D" python3 -I scripts/test_reconcile.py   # 29 tests
```

`fetch_inputs.sh` accepts `MOUSEION_REPO`, `TREEBANK_REPO` and `CANONICAL_REPO` overrides pointing at existing clones; the commits and hashes are still checked.
