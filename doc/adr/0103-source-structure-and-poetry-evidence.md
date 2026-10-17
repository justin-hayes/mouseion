# ADR 0103: Version source structure without rewriting frozen evidence

Status: **Accepted — implementation pending** · Date: 2026-10-17 · Author: Justin + OpenCode

Accepted through [Decision: accept the Faust I poetry specification and
handoff](https://github.com/justin-hayes/mouseion/issues/2151). Behavior and release
checks live in [Basic poetry support](../features/basic-poetry-support.md).
The earlier structure, context, compatibility and presentation decisions remain
linked there as provenance; this ADR records the promoted architecture and the
consequential shape deferred to this ticket, not new decision ownership.

## Context and trade-off

EPUB extraction currently conflates authorial `<br>` with block boundaries and
loses markup roles. A display-only fix cannot restore cross-line sentence
context. Normalizing a second NLP text or rewriting old snapshots would break
source-slice alignment, exact provider/cache identity, occurrence corrections
and frozen Reading/deck evidence. New extraction must coexist with old evidence
under the same acquired bytes, without mass-switching the learner's Books.

The chosen trade-off retains one authoritative text, adds immutable snapshot
structure and an explicit adoption path, and accepts analyzer/markup limits.
It extends the EPUB extraction/snapshot contract, preserves ADRs 0013,
0059/0060 and 0066, and extends ADR 0071's frozen input set. It amends ADR 0086's
universal Study affordance only for sentences overlapping speaker labels and
extends ADR 0087's shared eligibility inputs, not its Go-only ownership.

## Extraction and offset contract

- **Snapshot schema v2:** `<br>` is `\n`; source block/blank-line boundaries are
  `\n\n`; incidental whitespace collapses as before. NLP receives the exact unit
  text through the existing proto. Corpus sentence/token tables retain their
  current shape. Source lines/group membership are derived from unit offsets.
- Record the versioned markup convention-set identity on the snapshot. Its
  initial allow-list is specified in the feature; never infer roles or genre
  from typography, capitalization or words.
- Add **immutable relational role-span rows per unit**, owner/source/snapshot/unit
  scoped, with role and nonempty half-open code-point start/end. Allow proper
  nesting (including a direction inside a label); validate roles, bounds and
  non-contradictory structure. Do not copy role text or persist speaker identities.
- Record **immutable per-unit heading-reset offsets** as snapshot metadata,
  with the same ownership/reference and bounds validation. Existing navigation
  labels do not locate every heading inside a unit. These offsets reset derived
  attribution; a heading is not a third vocabulary structural role.
- Every unit/role/reset reference resolves through the exact snapshot, including
  frozen evidence. Unit IDs may recur across snapshots; they are not sufficient
  identity on their own.
- A content revision still owns original bytes/digest. Its historical `full_text`
  cannot validate a later extraction. Obtain snapshot text by joining that
  snapshot's ordered immutable unit texts with the established separators;
  validate unit spans against that result, never a different extraction's text.
  Do not mutate revision text or introduce a second analysis representation.
- Readers support v1 and v2 separately from the current writer version. v1 is
  **structure not recorded**; empty role data on v1 does not establish prose or
  absence of labels. Never infer source lines from v1 paragraph separators.

## Durable checks and adoption

Add a durable, owner/source-scoped check record for the base content revision
and snapshot, target contract and convention set. It records successful outcome
**unchanged**, **update available**, or **original unavailable**, and enough
deterministic result identity to verify the offered extraction on confirmation.
The comparison covers source text, roles and relevant structural/provenance
metadata, not screen presentation. Store no candidate snapshot before adoption.

River owns queued/running/retry/failure execution, rather than a parallel durable
job state machine. A crash or failure cannot create a successful check result or
partial snapshot; failed work is retryable and cannot mean unchanged. Enqueue
eligible checks through a bounded startup scan and relevant source-evidence
changes. Reuse successful exact-key checks; after River exhausts its bounded
attempts, retain explicit retry. No continuous rescanning, new scheduler, NLP in
the check, automatic reanalysis or data-migration-enqueued adoption.

For a v1 extraction whose text is identical and has no roles, recording a
successful check is a no-op: the current snapshot/analysis remain current.
Missing retained bytes instead produce original unavailable; old evidence stays
usable and adoption of that legacy record is unsupported. Do not automatically
download or promise that idempotent ordinary acquisition repairs it. Imported
analyses are excluded.

**Learner confirmation** carries the checked base revision/snapshot and target
identity. Re-extract retained bytes and verify the checked result. In the
owner/source transaction, revalidate/lock the expected current source, create or
reuse the exact immutable snapshot, switch the current pointer, and enqueue its
analysis atomically. A concurrent source change invalidates the offer; refresh
the check instead of silently adopting different evidence. Extraction may run
outside the transaction, but the final expected-source guard cannot.

Enforce at most one new snapshot per **(content revision, extraction contract
version, convention-set identity)** within its owner/source scope. Apply the
uniqueness constraint to the new contract without deduplicating or rewriting
historical v1 snapshots. Analysis-run deduplication includes snapshot identity
alongside its existing configuration/source inputs and serialization: double
confirmation and job retry converge on the same run. Publication continues to
require the run's exact snapshot be current.

The switch immediately makes a mismatching published analysis stale; existing
current-count/readiness rules apply until reanalysis publishes. Failure after
the switch uses existing failed/stale analysis and Re-analyze recovery. There
is no revert-to-v1 action. Later convention-set changes take the same check,
offer, snapshot and reanalysis path, including role-only differences; no
cross-snapshot corpus reuse.

## Vocabulary and frozen boundaries

Structural speaker exclusion is applied by Go `selection` using exact-snapshot
spans, before an occurrence can contribute an identity or denominator token.
SQL fetches structural facts but never reimplements eligibility. Follow
ADR 0087's builder version/readiness/rebuild contract for the changed rule;
unchanged prose's resulting counts must remain identical. A projection rebuild
is not a source switch, NLP run or mutation of a Reading freeze.

Keep raw corpus evidence, including labels. A sentence overlapping a label is
not study/card context and has no Study affordance, even when other tokens are
eligible stage-direction text. This avoids changing analyzer boundaries or
deleting part of a complete source sentence. Other directions follow ordinary
eligibility. Nested direction text is excluded with an enclosing speaker label.

Adoption never remaps corrections or rewrites Reading vocabulary, Reserved or
Known facts, completed history, corpora or deck generations. Active Reading can
coexist with the switch but stays on its frozen evidence. New evidence requires
End/restart and re-preparation; confirmation explains this and the count of
corrections/exclusions that will not apply to the new analysis.

## Deck specification and presentation

Extend the new manifest with optional typed attribution: a speaker name, or
stage-direction status. Derive it from the exact source snapshot during
preparation; omit nested directions from the displayed speaker name. Freeze it
in the presentation-independent input set and its versioned digest/codec.
Advance **manifest schema**, **render-input version** and **presentation
version** for their respective changes; their numbers are not extraction
contract or analysis versions.

Old manifests/codecs/digests stay readable and are never backfilled. Their
absent optional attribution means render without it, not fetch it from current
source or require re-preparation solely for this field. Preserve existing
mandatory render-input completeness/recovery rules. Append the Anki
`Attribution` field after existing fields, with explicit per-line markup in
`Text`; preserve note/model identities for the same specification. Real Anki
import/re-import compatibility is a release gate, not an assumption based on
package inspection. A new specification may legitimately change card identity.

Presentation-only rebuilding never reselects/regrades or calls NLP/providers.
Exact newline-bearing sentence hashes remain distinct from old contexts; new
evidence/attribution requires explicit re-preparation, not cache normalization.

## Rollout, recovery and governance

Implement with new additive successor migrations under ADRs 0038/0070, never
edit the baseline or shipped migrations. Old evidence and references are
retained; the learner-surfaced check is not a content backfill. Do not rewrite
legacy snapshots merely to populate new provenance. Physical SQL names,
indexes and validation mechanics implement the accepted ownership, constraints
and lifecycle above; they must not decide another product boundary.

Use a **single-version cutover**: stop old application/worker processes, apply
additive DDL, then start code that reads both contracts before writing v2.
Mixed old/new writers and downgrade after v2 adoption are unsupported. Recover
through retry/forward fixes, not a destructive down migration or an implied
source revert. Human CODEOWNER review is required for ADR/index and migration
PRs. This ADR authorizes no production operation or implementation execution.

## Rejected alternatives and costs

- A second normalized analysis text/offset map, per-line text records, genre
  flags, inferred roles and metrical lines add models without meeting the
  exact-source requirement more simply.
- Roles stored as an opaque unit JSON field would make query-time structural
  facts less explicit; relational spans keep constraints and exact references
  visible. Per-line corpus tables are unnecessary.
- In-place extraction, correction remapping, automatic library-wide adoption,
  staged snapshot-plus-analysis promotion and revert-to-v1 conflict with the
  accepted explicit switch/frozen-evidence boundary.
- Persisting candidate snapshots before confirmation adds durable extraction
  artifacts for unaccepted offers; recomputing a bounded extraction is simpler.
- The cost is more immutable source metadata, an extra extraction on confirmation,
  snapshot-aware idempotency, explicit learner restart, honest omissions and
  imperfect historical German analysis. Missing bytes and cross-unit/chunk
  context are deliberately bounded, not hidden future requirements.
