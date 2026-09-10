# ADR 0034: One implicit Reading Journey with learner-canonical ordering and campaign-queue migration

Status: **Accepted** · Date: 2026-08-31 · Author: Justin + Hermes

Supersedes the learner-facing queue semantics of [ADR 0027](0027-learning-campaigns.md)
while retaining ADR 0027's Campaign object and vocabulary reservation/graduation
contract as history and internal domain state. Recorded for issue #461 as part of
the My Books / Reading Journey / Primary Goal migration (#480).

## Context

The accepted learner-facing architecture (merged PR #459 and
[`doc/design/information-architecture.md`](../design/information-architecture.md))
defines **Reading Journey** as a fluid, provisional ordering of learner-selected
books the learner currently imagines reading. It is explicitly not a queue,
curriculum, project plan, calendar, or promise to finish the sequence: it has no
destination, dates, overdue state, progress percentage, or completion state, and
no automatic next Primary Goal.

Today the learner-facing plan is the **Campaign queue**, derived from queued
`learning_campaigns` rows ordered by creation (`migration 000021`). ADR 0027's
"one active campaign at a time, a queue of future books" is a commitment-shaped
model. The accepted Journey is a preference-shaped model. They cannot coexist as
two learner-facing plans: `/campaigns` currently surfaces the queue while the
canonical architecture makes Reading Journey the primary destination.

This decision fixes Journey identity and membership, ordering semantics,
concurrency and stale-write behavior, and the migration from Campaign
queue/history, so a later schema/UI milestone can implement it without inventing
a competing plan.

## Decision

### Identity: one implicit Journey per owner

There is exactly **one implicit Reading Journey per learner**, scoped by the
learner account (`owner_id`). It is not a named, explicit aggregate: the Journey
has no row of its own carrying a display name, description, or destination. Its
identity is the owner plus the ordered membership set.

Multiple named Journeys are explicitly **out of scope** unless a future decision
separately accepts them. A learner with an empty Journey simply has no membership
rows; the empty state is ordinary and must not be rendered as a failed or
completed plan.

### Owner-scoped membership

Membership is an owner-scoped set of book entries: `(owner_id, book_id)` is
unique. A book may be a Journey member independently of whether it is acquired
(B1 contract), analyzed, has a prepared deck, or is a Primary Goal. Membership is
learner-selected only: nothing auto-adds or auto-removes a book. Adding to the
Journey never starts analysis or deck preparation, and never reserves
vocabulary.

### Ordering: learner order is canonical and deterministic

Learner order is the **authoritative** order (requirement 7 and the migration
plan's invariant). An efficient/optimized order is advisory comparison evidence
only (separate cross-book projection contract) and never becomes the stored
order.

The membership table stores an explicit integer `position` per owner with a
deterministic, stable sort:

```text
(owner_id, position, created_at, book_id)  -- chosen comparator
```

`position` is the learner-facing **Your order** and is kept contiguous (with
consecutive integers) by the mutation that changes it. `created_at` and `book_id`
act only as a deterministic tie-break for rows whose `position` is momentarily
equal during a concurrent reorder, so reads are always stable and reproducible
even under write contention. The learner-facing "Move earlier / Move later"
controls mutate a single row at a time and are the supported interaction; drag may
enhance but never replaces them.

### Concurrency and stale writes

Journey mutations are **per-owner serialized**: only one in-flight membership or
order mutation for an owner is accepted at a time, and each operates on a read of
the owner's current membership set. Because the list is small and learner-led,
the implementer uses optimistic concurrency on the owner's membership table
(a per-owner `updated_at` or monotonic revision), and re-orders one row at a time
with the deterministic comparator above as the tie-break. The resulting behavior:

- Concurrent inserts that target the same owner are reconciled deterministically:
  a fresh read happens before the write commits; the tie-break makes both
  converges to a stable order.
- A stale write (a mutation based on an outdated snapshot) is **rejected** with a
  conflict and the client refetches rather than silently overwriting a newer
  learner order. Learner preference is never rolled back silently.
- After any reorder, preparation/comparison evidence is recalculated with neutral
  language ("changes the modeled preparation by …"). A failed recalculation
  preserves the accepted order and existing trustworthy evidence, explains the
  failure, and offers retry — it never rolls the order back.

### Relationships

**My Books.** My Books is the broad owner-scoped catalogue; Reading Journey is an
ordered subset of it. Membership references the same owner-scoped book identity
(see the My Books membership B1 contract). A book can exist in My Books outside
the Journey, and a Journey entry never represents a book the owner does not
possess/desire in My Books.

**Primary Goal.** Primary Goal is a separate contract (B3) not decided here. A
Journey entry is independent of Goal role: the current Primary Goal is anchored
in the Journey's presentation, but choosing/changing/clearing a Goal does not add,
remove, or reorder Journey membership, and Journey membership never implies
commitment. The first provisional book is a candidate for a future Goal, never an
automatic one.

**Prepared decks.** A prepared deck is an immutable owner-scoped APKG artifact
bound to one analysis result. Journey membership does not require a prepared
deck. A ready deck may offer **Add to Reading Journey** (the deck→Journey CTA,
B8), but deck readiness is not a precondition for membership, and membership never
marks vocabulary known or reserves it.

**Campaign history.** ADR 0027's Campaign object remains the internal
reservation/graduation mechanism and its completed/abandoned rows remain
immutable history kept for reading, preparation, and vocabulary-transition
provenance. Campaign records are never deleted or rewritten to fit the Journey,
and Campaign never returns as a learner-facing plan.

### Migration: queued / active / complete / abandoned

On introducing the Journey table, existing `learning_campaigns` rows migrate as
follows (a single explicit data migration; no rewrite or deletion of immutable
provenance):

| Campaign status | Migration | Destination |
|---|---|---|
| `queued` | Insert a Journey entry for each queued campaign's book, preserving creation order as the initial `position`. | Reading Journey |
| `active` | Keep as the current active Campaign; do **not** auto-insert into Journey membership (Goal/active handling is the separate B3 contract). | Campaign (active) |
| `complete` | Leave as-is; it becomes history only. | Campaign history |
| `abandoned` | Leave as-is; abandoned vocabulary becomes eligible again per ADR 0027, but the record stays history. | Campaign history |

Completed and abandoned work **remains history** and never re-enters Reading
Journey. `learning_campaign_vocabulary` provenance and known-vocabulary rows are
untouched. The Campaign object remains authoritative for reservation/graduation
semantics during and after the transition.

### `/campaigns` retirement and duplicate-queue removal

The staged rollout described below reached its webapp retirement step in T3
(`#678`). The `/campaigns` surface is no longer a learner-facing route and
**must not remain a duplicate queue** alongside Reading Journey:

- `GET /campaigns` and campaign mutation routes are unregistered from the
  webapp and return 404.
- Existing deep links into the retired queue surface are not rendered; learners
  use the corresponding Journey entry or My Books surface instead.
- The queue-derived ordering is removed; the Journey table becomes the single
  authoritative learner order.
- Historical Campaign records remain internal backend state for migration and
  provenance; they do not create a learner-facing history or operations surface.

### Staged rollout consistent with #449

Rollout is staged in additive steps so each migration PR is independently
landable (consistent with the schema-change governance and staged-rollout policy
#449): (1) add the Journey membership/order tables (additive, non-destructive),
(2) backfill the queued-campaign migration as a separate, idempotent data step,
(3) introduce the Reading Journey surface over the new tables, (4) redirect and
retire `/campaigns`, (5) reconcile docs/tests/terminology. New-SQL-migration PRs
are CODEOWNER-blocked by repo policy and must be merged manually; that is not a
CI failure. The active-Campaign→Primary Goal path lands with B3, after this
decision.

## Alternatives considered

- **Explicit named Journey aggregate.** Adds a Journey row with identity/name;
  rejected because it is unnecessary for one implicit Journey and invites the
  multi-Journey expansion that is explicitly out of scope.
- **Queue as a separate table under a new name.** Rejected: this preserves queue
  semantics under different labels and guarantees two learner-facing plans,
  which the non-goals forbid.
- **Float/lexorank positions for reordering without full re-numbering.** Rejected
  for "Move earlier / Move later" granularity: the learner list is small, single
  moves are fine to re-number, and dense integers make the canonical order
  directly readable and testable.
- **Last-write-wins concurrency.** Rejected: a silent overwrite of a learner's
  reorder across concurrent tabs is unacceptable; optimistic per-owner conflict
  is cheap and preserves the canonical order as authoritative.
- **Auto-migrate `active` campaigns into the Journey head.** Rejected: ties Goal
  (a commitment) to Journey membership (a preference) before B3 is decided and
  would conflate two contracts.
- **Deleting or compacting `learning_campaigns` history.** Rejected: ADR 0027
  provenance and graduation data must remain immutable; only learner-facing
  presentation changes.

## Consequences

### Positive

- One learner-facing plan; Campaign queue is definitively not secondary.
- Learner order is authoritative and deterministic under concurrency.
- Campaign history and vocabulary provenance remain intact and authoritative.
- Migration is additive and staged, matching the #449 schema-governance defaults.
- Decks, My Books, and Primary Goal relationships are explicit and decoupled.

### Costs

- A Journey membership/order table and lifecycle UI are required.
- Optimistic per-owner concurrency and deterministic reordering must be
  implemented and tested.
- `/campaigns` redirect/deep-link handling must be additive with the route and
  terminology rollout (B7), not bolted on.
- Existing generated-vocabulary and campaign history need their compatibility
  behavior preserved before selection changes (ADR 0027 compatibility note).

## Non-goals

- No schema/UI implementation in this decision.
- No destination, deadline, Journey completion, or automatic next Goal.
- No cross-book optimization algorithm; efficient ordering is advisory only and
  covered by a separate projection contract.
- No queue rename that preserves queue semantics.
- No deletion, rewrite, or consolidation of `learning_campaigns`,
  `learning_campaign_vocabulary`, or known-vocabulary provenance.

## Related

- Issue #461 (this decision); part of #480.
- [`doc/design/information-architecture.md`](../design/information-architecture.md)
  (canonical direction; Journey identity, membership, ordering, and queue-replacement
  contract items).
- [`doc/design/workflows/learning-campaign.md`](../design/workflows/learning-campaign.md).
- [ADR 0027](0027-learning-campaigns.md) (superseded learner-facing queue; retained
  Campaign contract).
- [ADR 0019](0019-generated-vocabulary-exclusion.md) and ADR 0027 compatibility note.
- Schema-change governance / staged rollout #449; rollout tracking #480.
