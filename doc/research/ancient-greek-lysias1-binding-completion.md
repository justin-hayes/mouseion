# Lysias I binding and segmentation completion (#2048)

**Status: FAIL, unchanged. No route passes, and this work adds no pass.** The producer-tokenizer evidence explains the sentence-11 enclitic order under one historical rule, but producer identity is not established, so sentence 11 stays blocked. The sentence-125 route is a decision for the parent, not a finding. The ledger overlay changes 30 rows, adds two blocker labels to them, and changes no readiness state.

This is a bounded subagent pass over text binding and segmentation for the reconciliation at [`7ac01256`](https://github.com/justin-hayes/mouseion/commit/7ac012569e72ae1ed9d5dec5fc1ef521f8cbe65d). It reads the baseline manifests and does not repeat their investigations. It does not decide which Greek reading is correct, does not adopt a source or route, and does not measure #2047. Lexical evidence for #2048 is owned by a separate agent. Mapping conventions stay with #2049. Final acceptance stays with #2030.

Artifacts are in [`ancient-greek-lysias1-binding-completion/`](ancient-greek-lysias1-binding-completion/README.md). Rights are in [`NOTICE.md`](ancient-greek-lysias1-binding-completion/NOTICE.md).

## 0. Per-layer result

Each layer is judged on its own. "Unassessed" is not "pass".

| Layer | Result | Evidence | Still open |
|---|---|---|---|
| Authority (which printed text a route reads) | Unchanged from baseline. T1A2 reads grc2 for text and AGDT v1.6 for annotation. | Baseline report §1; `s125w015` witness split (§3). | Route decision at `s125w015` (`BD-S125-ROUTE`). |
| Binding (node to canonical span) | Sentence-11 nodes stay bound to their shared `μήτε` tokens (`cg2:w0264`, `cg2:w0266`). Their component order is not settled. | Baseline `nodes.tsv` `b_*` columns; ledger rows. | Order is conditional on producer identity (`BP-PRODUCER-IDENTITY`). |
| Segmentation (producer token boundaries) | Under the 2014-08-18 rule (`c9b60a34`), the historical producer reproduces the native enclitic split in sentence 11 exactly. Under the 2014-08-25 rule (`a4a810f`), it contradicts 11 native tokens. The native annotation matches the older rule only. | §2, `manifest/producer_*`. | Producer identity is not proven, so this is evidence about the annotation, not about a verified producer. |
| Provenance (who produced the native segmentation) | **Not established.** The service named in the AGDT v1.6 header returns HTTP 500, and no public source links it to these tokenizer commits. | §2.3. | Any source that ties `llt/segtok` to a revision. |
| Licence (rights in inputs and outputs) | Tokenizer is MIT (LFDM 2013), read as data and not vendored. Other inputs as in the reconciliation NOTICE. | `NOTICE.md`. | The same open items as the reconciliation. |

## 1. Scope, inputs and method

- **Immutable baseline:** `7ac012569e72ae1ed9d5dec5fc1ef521f8cbe65d`. The baseline files were hashed from the git blobs at that commit and match: `route_nodes.tsv` `a2b8b427…`, `sentences.tsv` `57ec6f4d…`.
- **Native annotation:** AGDT v1.6 Lysias I (`treebank_data@bf4334f0`). 2,802 printed nodes in 134 sentences; the baseline counts 2,834 nodes including unprinted ones.
- **Canonical text:** grc2 (`canonical-greekLit@01b725d8`, pinned); grc1 at `aa0dd9e6`, the historical blob that the producer probe reads (see §2).
- **Producer candidates:** llt-tokenizer `c9b60a345708af72c721ec1183c9ab928f7b8c03` (2014-08-18, the last commit before the AGDT v1.6 date stamp of 2014-08-21) and `a4a810f02f73bacf3f16e722425dfb80f3b2a5ab` (2014-08-25, the first commit to change the accent on `οὔτε`). Both are read from the pinned commits with `git show`, never from a worktree head.

No witness was fetched again. No restricted source was retried. No broad source survey was run.

## 2. Task A: producer-tokenizer evidence

### 2.1 What the probe does

`scripts/producer_probe.py` reproduces the Greek path of the historical tokenizer: `split_krasis`, the enclitic map (split off and inserted before the host), and PUNCTUATION splitting. Latin-only steps cannot touch Greek running text and are not run. The Ruby sources are parsed as data. Character lists and regex literals are not retyped, so the probe reproduces the source's own typos, including the Latin `ó`, `ì`, `ò` in `greek.rb`. Its effect on the `ALL*` krasis match has not been tested.

### 2.2 Results

| Text | Revision | Sim tokens | Equal tokens | Non-equal ops | Enclitic insertions |
|---|---|---|---|---|---|
| grc1 @`aa0dd9e6` | `c9b60a34` (2014-08-18) | 2,797 | 2,795 | **7** | **2** |
| grc1 @`aa0dd9e6` | `a4a810f` (2014-08-25) | 2,808 | 2,784 | 18 | 13 |
| grc2 | `c9b60a34` | 2,796 | 2,795 | 7 | 2 |
| grc2 | `a4a810f` | 2,807 | 2,784 | 18 | 13 |

Native: 2,802 printed nodes. Full alignments are in `manifest/producer_alignment_*.tsv`; insertions in `manifest/producer_enclitic_insertions_*.tsv`.

**Under `c9b60a34`:**

- The two insertions are both `-τε` before `μή`, at sentence 11. They match the native nodes `s011w025` and `s011w028` (`-τε`) and `s011w026` and `s011w029` (`μή`), which the alignment shows as equal.
- The 11 krasis fragments (22 nodes, `τ-`/`κ-` before the host) reproduce with no reordering.
- The 7 non-equal ops against grc1 2014 are: `s060w009` (native `ἡμέρᾳ` U+1FB3, grc1 2014 `ἡμέρα`; grc2 matches native); `s125w015` (native `οὔτε` U+1F54, grc1 2014 `οὕτε` U+1F55); and five AuxK stops, `s078w002`, `s082w002`, `s084w002`, `s090w002` and `s121w002`, which the native annotation prints and the 2014 text does not. These match the baseline statuses: `s060w009` is exact against grc2; `s125w015` is `variant_positional`; the five stops are `annotation_only_print_attested` and `punct_blocked`.
- Against grc2, the 7 ops are `s002w023` (deleted in grc2, which the baseline records as `annotation_only_deleted_in_grc2`), `s125w015`, and the five stops.

**Under `a4a810f` (the 2014-08-25 change):**

- It adds 11 splits that contradict native tokens: `οὔτε` at `s008w041`, `s008w051`, `s008w063`, `s075w030`, `s075w032`, `s075w034`, `s125w001`, `s125w008`, `s125w020` and `s125w034`, and `οὐδὲ` at `s127w016`. The native annotation keeps each of these whole.
- So native v1.6 is consistent only with the pre-08-25 behaviour. This is evidence about the annotation's source, not about which producer made it.

### 2.3 What is not shown

- **Producer identity is not proven.** The AGDT v1.6 header names `services.perseids.org/llt/segtok`, which returns HTTP 500, and no public source links it to these commits. Treat `c9b60a34` as a plausible baseline, not as the deployed producer.
- **The producer input is not the route text.** The probe reads grc1 at `aa0dd9e6`. It differs from grc2 at `s060w009` and `s125w015`. The probe's agreement is therefore evidence about a 2014 text, not about the route text. T1A2's exact status for `s060w009` is against grc2, and that stands.
- **Elisions** (U+02BC) are not split by the producer, since PUNCTUATION does not include ʼ. Elision handling is unchanged from the baseline.

## 3. Task B: sentence 125, `s125w015`

The native annotation prints `οὔτε` with smooth breathing (U+1F54). The canonical witnesses print rough breathing (U+1F55).

| Witness | Form at `s125w015` |
|---|---|
| AGDT v1.6 (native annotation) | smooth, U+1F54 |
| Mnemosyne (MN) | smooth |
| Wikisource (WS) | smooth |
| Hude (crop) | smooth |
| grc1 @`aa0dd9e6` (2014 producer input) | rough, U+1F55 |
| grc2 (pinned route text) | rough, U+1F55 |

The text side is resolved: the variant is real and the baseline already records it as `variant_positional`. The route decision is open. ADR 0093 allows annotation-carried text to be authority when verified, and forbids silent substitution for another edition.

**Options:**

- **A. Keep blocked.** T1A2 keeps grc2 as text, and `s125w015` stays a recorded variant (`BT-S125-VARIANT` plus `BD-S125-ROUTE`). No route change. This is the current state.
- **B. Accept the annotation's smooth form for this node,** recording it as an explicit variant against grc2. This follows the ADR 0093 route for annotation-carried text, but it changes the authority for one printed node, so it needs an explicit decision.
- **C. Adopt grc2's rough form silently.** Not recommended: it would replace the annotation's printed form without a record.

**My recommendation is A**, until the parent decides whether the annotation may serve as text authority at a single node. If the parent chooses B, it must be a recorded variant. This is a recommendation, not a decision. No route or source is adopted here.

## 4. Task C: segmentation and binding ledger

### 4.1 Method

`scripts/completion_ledger.py` overlays the completion proposals on `route_nodes.tsv` at `7ac01256` (6 routes × 2,834 rows). Its rules are narrow:

- It changes no `text_status`, `text_rule`, `vocab_class`, `identity_status`, discriminator field or `readiness`.
- It never removes a blocker. It adds one label to each affected row and writes two proposal columns: `completion_proposal` and `completion_change`.

The proposal for sentence 11 is a conditional shared span on the two `μήτε` canonical tokens, with the component order taken from the historical producer rule. For `s125w015` it is a decision request. Neither is a pass.

### 4.2 Changes

| Set | Nodes | Rows | Added label | Readiness after |
|---|---|---|---|---|
| Sentence 11, `μήτε` pairs | `s011w025`, `s011w026`, `s011w028`, `s011w029` | 24 (4 nodes × 6 routes) | `BP-PRODUCER-IDENTITY` | Unchanged |
| Sentence 125, `οὔτε` | `s125w015` | 6 (1 node × 6 routes) | `BD-S125-ROUTE` | Unchanged |

Sentence-level totals are in `manifest/completion_sentences.tsv`. Changed rows are in `manifest/completion_changes.tsv`.

Readiness across all 17,004 rows is unchanged, and the counts match the baseline:

| Readiness | Rows |
|---|---|
| vocab_ready | 3,457 |
| vocab_blocked | 4,913 |
| non_vocab_ok | 3,042 |
| non_vocab_blocked | 3,156 |
| punct_ok | 1,104 |
| punct_blocked | 1,140 |
| unprinted | 192 |

Ready claims added: **0**.

### 4.3 Node status under T1A2

| Node | Native | T1A2 baseline status | Readiness | Blockers (after) |
|---|---|---|---|---|
| `s011w025` | `-τε` (AuxY, head 28) | `blocked_component_order` | `non_vocab_blocked` | `BT-S011-ORDER;BP-PRODUCER-IDENTITY` |
| `s011w026` | `μή` (AuxZ, head 27) | `blocked_component_order` | `vocab_blocked` | `BT-S011-ORDER;BI-ELIG-PARTICLE;BP-PRODUCER-IDENTITY` |
| `s011w028` | `-τε` (COORD, head 24) | `blocked_component_order` | `non_vocab_blocked` | `BT-S011-ORDER;BP-PRODUCER-IDENTITY` |
| `s011w029` | `μή` (AuxZ, head 33) | `blocked_component_order` | `vocab_blocked` | `BT-S011-ORDER;BI-ELIG-PARTICLE;BP-PRODUCER-IDENTITY` |
| `s125w015` | `οὔτε` | `variant_positional` | `non_vocab_blocked` | `BT-S125-VARIANT;BD-S125-ROUTE` |

The T1A2 status of the other sentence-11 nodes is unchanged and not listed here.

The producer's seven non-equal ops are mapped to their T1A2 statuses in `manifest/completion_producer_ops.tsv`. Every one of them is either already blocked, or is `s060w009`, which is already exact against grc2.

### 4.4 Machine checks

`manifest/completion_checks.json` reports `all_checks_pass: true`:

- 6 routes, 2,834 nodes each, unique keys per route; 134 sentences, node total 2,834.
- The row set is identical to the baseline, and locked fields are unchanged across all rows.
- Blockers only grow.
- Every changed row gained its labelled blocker and is still blocked.
- 24 sentence-11 rows and 6 `s125w015` rows, as expected.
- Ready claims added: 0.
- Sha256 of the baseline inputs recorded.

The unit tests (`scripts/test_producer_probe.py`, `scripts/test_completion_ledger.py`) run 23 tests, including the full probe re-run against the recorded summary. All pass.

## 5. Blockers and what would settle them

| Label | Scope | What would settle it | Owner |
|---|---|---|---|
| `BP-PRODUCER-IDENTITY` | Sentence 11, 24 rows | Public evidence that `llt/segtok` (or the Perseids service) runs these tokenizer revisions. A reproduced service output for a sentence containing `μήτε` would also settle it. | Parent / #2048 |
| `BT-S011-ORDER` | Sentence 11, 24 rows | Same as above. The order can be accepted only with producer identity and the host-first practice question resolved together. | Parent / #2048 |
| `BD-S125-ROUTE` | `s125w015`, 6 rows | Parent decision among options A, B, C in §3. | Parent |
| `BT-S125-VARIANT` | `s125w015` | Same decision as above. | Parent |

The producer contradictions under `a4a810f` do not block the baseline, since the baseline does not rely on that revision. They are reported so that no one adopts that revision without accounting for 11 native tokens.

## 6. Rights

See [`NOTICE.md`](ancient-greek-lysias1-binding-completion/NOTICE.md). In summary: the llt-tokenizer is MIT (LFDM 2013) and is read as data, not vendored. The producer's rights are not stated because its identity is not established. Other inputs keep the reconciliation's open items.

## 7. Reproduce

See [`README.md`](ancient-greek-lysias1-binding-completion/README.md). The probe and the ledger both run from the committed scripts, and the ledger's `manifest/` outputs regenerate byte-identically.

## 8. Choices made here

- **Sentence-11 status.** I kept the baseline `text_status` and readiness, and added a blocker, instead of changing status to a "conditional" value. That keeps the table's vocabulary unchanged and makes it impossible for the overlay to pass a node.
- **Blocker labels.** `BP-PRODUCER-IDENTITY` and `BD-S125-ROUTE` are research-local labels added here. They are not defined in the baseline vocabulary.
- **Producer baseline.** I used `c9b60a34` as the candidate, because it reproduces native sentence 11 exactly and the post-fix revision does not. This is a choice of test, not evidence of identity.
- **No raw Greek in manifests.** The probe escapes every form as code points, and the ledger's proposal text is ASCII, to follow the reconciliation NOTICE.
- **Scratch.** The worktree and inputs are retained for parent review under `~/.cache/scratch/greek-lysias1-binding-completion/`. One throwaway file written under `/tmp` during the test run was deleted immediately.
