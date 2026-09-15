# ADR 0038: Schema-change governance and migration review policy

Status: **Partially superseded by [ADR 0070](0070-migration-and-documentation-reboot.md)** (immutability clause only) · Date: 2026-09-01 · Author: Justin + Hermes

Records the schema-change decision and review gates for issue #449, part of the
Experience architecture migration (#480). This ADR governs readiness and
rollout decisions; it does not prescribe a particular SQL implementation.

## Context

Mouseion's paired `golang-migrate` files are deployment history. Short-lived
schema decisions such as `000029_long_context` followed by
`000032_remove_long_context`, and data-only backfills such as
`000013_backfill_generated_vocabulary`, show why durable shape should be
decided explicitly and why data movement needs different treatment from DDL.
The admin-role reversal and EPUB classification correction provide further
rationale.

These are historical rationale only. They do not condemn valid shipped state or
require any retroactive migration changes.

The important boundary is between two kinds of work:

- **Decision readiness** establishes product behavior, domain shape, lifecycle,
  ownership, compatibility, and rollout policy in an accepted issue, feature
  document, or ADR.
- **SQL implementation** turns that shape into tables, fields, constraints,
  indexes, backfills, and deployment steps in the migration PR and its tests.

SQL detail must not silently decide an unsettled product or architecture
question. Conversely, a small additive implementation with an already clear
meaning should not require a heavyweight architecture process merely because it
touches a table.

## Decision

### Shipped migration history is immutable

Every shipped `0000NN_*.sql` migration is immutable deployment history. Shipped
migrations must not be edited, deleted, renumbered, squashed, or consolidated.
Corrections to shipped behavior use a new migration with an explicit rationale;
they do not rewrite the historical record. Down migrations are not presumed to
be safe, complete, or suitable for destructive production rollback.

### Settle consequential shape before SQL

Before a migration PR lands, an accepted issue, feature document, or ADR must
record the shape and rollout decision for:

- a non-trivial new table or column;
- a new or materially changed constraint or relationship;
- a destructive change, including removal or narrowing of stored data; or
- a durable state machine, lifecycle, ownership rule, or other persistent
  policy encoded by the schema.

The decision must explain the data's meaning, ownership, states and transitions,
compatibility, rationale, and a rollout/recovery approach appropriate to the
data volume and compatibility boundary. A migration implements that decision;
it does not resolve an unrecorded product or architecture choice.

### Narrow exception for low-risk additive fields

An additive, non-breaking field may use the ordinary issue and migration review
path without a separate heavyweight ADR only when all conditions hold:

1. It has one clear consumer and a plainly defined meaning.
2. Existing rows and readers remain valid with the field absent or at a
   compatible default.
3. It introduces no lifecycle, ownership, authorization, retention, or other
   policy ambiguity.
4. It does not imply a parallel replacement for an unsettled or soon-to-be-removed
   table, column, or state model.

Normal review, compatibility notes, and tests still apply. A field that fails any
condition follows the consequential-shape gate; “additive” describes the SQL
operation, not whether the product decision is low risk.

### Resolve in-development shapes before merge

When an unshipped or in-development table, column, or relationship is likely to
be replaced or removed soon, resolve or replace it before merging its migration
whenever practical. Do not land a parallel shape to avoid the underlying
product or architecture decision. Credible near-term reversion risk stops
review until the decision is resolved and recorded, or the risky shape is
removed. This avoids churn without revising immutable shipped history.

### Separate structural DDL from data-only backfills

Data-only backfills should be separate from structural DDL when practical. The
backfill plan must document:

- the owner responsible for running and observing it;
- whether it is run once or idempotent;
- retry safety and duplicate-work behavior;
- transaction boundaries, locking, and expected impact;
- failure detection and recovery steps; and
- whether rollback is supported, or what forward fix is required.

Structural and backfill readiness may be reviewed together, but remain
distinguishable. A backfill does not make an unresolved shape acceptable, and
successful DDL does not establish that data movement is safely retryable.

### Stage changes when compatibility or volume requires it

Use expand/backfill/contract, or another staged rollout, when old and new
application versions must coexist, data volume makes one-step work risky, or
locking and availability require separation. Each stage needs a compatibility
boundary and completion signal before a dependent stage lands.

Irreversible or destructive changes require explicit rationale and documented
recovery/backup expectations before implementation. The documentation must say
what can actually be recovered and by which mechanism; it must not imply that a
down migration reverses production data loss.

## Review gates

Every schema-related review checks that the product/architecture shape is
accepted and linked before SQL work lands (or that the additive-field exception
applies), that reversion risk is resolved rather than encoded as a parallel
shape, and that any backfill has an operational contract. Staging,
compatibility, locking, recovery, and destructive-change expectations must be
explicit where applicable.

If a gate is not satisfied, review stops until the missing decision is recorded.
CODEOWNER review remains required for database migrations and other protected
paths under repository policy.

## Consequences

### Positive

- Durable product and architecture choices lead schema shape instead of being
  fixed accidentally by SQL.
- Small, clearly safe additive fields retain a proportionate path to delivery.
- Backfill ownership and operational behavior are visible before data changes.
- Staged rollouts and explicit recovery expectations reduce compatibility and
  irreversible-change surprises.
- Shipped migration history stays reproducible and auditable.

### Costs

- Consequential schema work may require an accepted decision before a migration
  PR can land; backfills need operational documentation and may require staged
  rollout, observation, or forward-fix tooling.
- Review must pause when reversion risk is credible, even if the SQL is easy to
  write.

## Non-goals

- No migration, schema change, application code, CI automation, or deployment
  change is created by this ADR.
- Not every additive field requires a heavyweight ADR.
- Down migrations are not declared universally safe; destructive production
  rollback is not presumed possible.
- Existing migrations are not edited, deleted, renumbered, squashed, or
  consolidated.

## Related

- Issue #449: schema governance and migration review policy; part of #480.
- [ADR 0024](0024-learner-owned-catalogs-no-admin.md) — durable domain policy
  reversal illustrating the value of explicit decisions.
- [ADR 0028](0028-explicit-scoped-analysis-lifecycle.md) — explicit lifecycle
  boundaries and immutable artifacts.
- [ADR 0034](0034-reading-journey-identity-ordering.md) — staged rollout
  consistent with this schema-governance policy.
- [ADR 0037](0037-cross-book-projection-advisory-ordering.md) — persistence
  remains subject to this governance if later chosen.
