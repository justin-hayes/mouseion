# ADR 0072: Goal-owned vocabulary snapshots and sequential Reading Journey forecast

Status: **Accepted** · Date: 2026-09-19 · Author: Justin + OpenCode

This decision supersedes the conflicting learner-state and coverage semantics in
[ADR 0036](0036-primary-goal-justified-graduation.md),
[ADR 0037](0037-cross-book-projection-advisory-ordering.md),
[ADR 0053](0053-book-anchored-vocabulary-consolidation.md),
[ADR 0019](0019-generated-vocabulary-exclusion.md), and the conflicting
learner-state exclusion clauses of [ADR 0025](0025-analysis-coverage-threshold-metrics.md).
It preserves the frequency
floor in [ADR 0048](0048-frequency-floor-deck-selection.md), the per-language
identity in [ADR 0051](0051-reading-journeys-and-goals-per-language.md), and the
reading-intent analysis trigger in [ADR 0049](0049-reading-intent-triggers-analysis.md).

## Context

The previous model made vocabulary reservation a separate Book/deck study
lifecycle. It required a study or review confirmation after deck preparation,
treated generated vocabulary as a cross-book exclusion, and offered a
vocabulary-efficient alternative order beside the learner's order. Those
concepts make Primary Goal, Reading Journey, deck preparation, and vocabulary
state compete as separate plans. They also make the useful ordering question
wrong: the learner needs to see what their chosen order means when they arrive
at each book, not be offered a second order to optimize.

The product needs one coherent learner model. A Primary Goal owns the active
vocabulary consequence for its Book. A Goal completion is the learner's
acceptance that the frozen recurring vocabulary should enter Mouseion's model of
known vocabulary. That is a modeled learner assertion, not a claim that the
learner has objectively mastered every identity or reviewed every card.

## Decision

### Goal and vocabulary ownership

There is at most one active Primary Goal per `(owner, study language)`. A Goal
may be chosen only for a Reading Journey member with a trustworthy completed
current analysis. Choosing it freezes the exact recurring-vocabulary identity
set selected from that analysis using the frequency floor and the current
eligibility rules. The snapshot is immutable for the lifetime of that Goal.

The active Goal is the sole owner of its snapshot's reservation. **Reserved
vocabulary** is derived from the active Goal snapshot, scoped by owner and study
language, and is not a separate learner-selected study state. It is excluded
from later recurring-vocabulary selection in that language but is not Known
vocabulary. Independent Goals and reservations in different study languages do
not block one another.

The snapshot keeps the exact analysis, source, and selection provenance needed to
explain and reproduce its identities. Reanalysis, catalogue changes, or deck
artifact retries do not change an existing Goal snapshot.

**Known vocabulary** is modeled learner knowledge. It consists of explicit
learner imports and identities accepted when a Primary Goal is completed. It
does not claim verified mastery, per-card review, or spaced-repetition success.
Known vocabulary remains owner- and language-scoped, uses existing lemma plus
UPOS and lemma-wildcard semantics, and uses set semantics when identities
overlap.

Generated vocabulary remains immutable provenance: it records that an identity
was included in a prepared deck and retains its Book/deck origin. Generation,
download, or historical assignment is neither Known nor Reserved and is not an
eligibility exclusion for a later Book. Selection excludes only current Known
vocabulary and the active Goal-derived Reserved vocabulary in the relevant
study language, while preserving the frequency floor, lexical identity, and
sentence-quality rules.

### Goal transitions

All Goal transitions are scoped to one owner and study language and are
serialized with the existing optimistic-concurrency rules.

**Choose.** Choosing a Goal atomically creates the active Goal and its frozen
snapshot. An empty recurring-vocabulary result is valid: the Goal remains
active, its snapshot is empty, and completion may add zero identities. Local
prepared-deck production may start from the same snapshot, but no external
translation consent is implied. A queued, failed, cancelled, or otherwise
unavailable deck artifact does not remove or alter the Goal or snapshot.

**Change.** Replacing a Goal releases the old Goal-derived reservation and
creates the new Goal and exact snapshot in one transaction. The old snapshot,
prepared artifacts, generated provenance, and operational history remain
historical. A failed replacement leaves the old Goal and reservation unchanged.

**Clear.** Clearing a Goal ends its active relationship and therefore releases
its derived reservation. It does not delete the snapshot, prepared deck,
generated provenance, or other historical evidence. Clearing does not graduate
anything and does not select another Goal.

**Complete.** Completing a Goal is an explicit consequential acceptance. One
atomic, idempotent transaction:

1. records an append-only, Book-anchored reading-completion fact;
2. inserts each frozen snapshot identity that is not already Known, retaining
   completion, Goal, Book, analysis, and snapshot provenance;
3. removes the finished Book from active Journey membership;
4. clears the active Goal and thereby releases its reservation.

The confirmation states the exact number of identities that can be added to
Known vocabulary. Repeating the accepted request returns the existing outcome
without duplicating completion or Known-vocabulary rows. A stale Goal identity
is rejected without partial writes. Completion never waits for a deck artifact,
requires deck review confirmation, or auto-selects a next Goal. An artifact
failure is therefore distinct from an empty snapshot and from completion state.

### Reading Journey forecast

The learner's stored Journey order is the only active order. The former
vocabulary-efficient alternative, route ranking, comparability totals, and
advisory ordering are retired from the Journey overview. Forecasts are derived
on demand and never persisted.

For a study language, let `K0` be current Known vocabulary. If an active Goal
exists, let `K1 = K0 union Reserved(Goal)`; otherwise `K1 = K0`. Each analyzed
Book in the learner's order exposes these distinct meanings:

- **Current coverage**: coverage using `K0` only.
- **After-Goal coverage**: coverage using `K1`, or the same value as current
  coverage when no Goal exists.
- **On-arrival coverage**: coverage using the accumulated modeled vocabulary
  immediately before the learner reaches that Book.

On-arrival calculation walks the learner's order. It starts with `K1`, reports
the current accumulated set for each Book, then models that Book's recurring
identities from its trustworthy current analysis after excluding the accumulated
Known/projected identities. Those identities are unioned into the accumulated
set for the next Book. Overlapping identities are counted once. Reordering
changes the affected forecasts only; it never changes Known vocabulary,
Reserved vocabulary, the active Goal, or membership other than the requested
order mutation.

Coverage keeps each Book's existing analyzable-token denominator and exact
integer arithmetic. Percentages are rounded only for display. Threshold
investment and highest-impact unknowns remain available on an individual
Journey entry, but threshold ranking and route-comparison methodology do not
appear on the Journey overview.

A Book with stale, unavailable, incomplete, failed, or otherwise untrustworthy
current analysis remains in the learner's chosen position and contributes no
modeled snapshot. It receives no fabricated zero or rank. If an earlier Book
cannot contribute a trustworthy snapshot, downstream modeled coverage based on
the contributions that are known is labeled a **lower bound**: the missing Book
may contribute additional identities, but the forecast does not invent them.
The same language and evidence rules apply whether or not a Goal exists.

### Migration and historical posture

Migration preserves historical Known vocabulary, prepared decks, generated
provenance, existing graduation provenance, and unconfirmed legacy work. No
unconfirmed legacy reservation or finished Goal is retroactively graduated.

Existing finished Goals are copied into durable reading history and removed from
active Goal/Journey state by a separate, owner- and language-scoped,
idempotent, retry-safe data backfill. A matching legacy active study may be
linked to the active Goal only when owner, language, Book, exact analysis, and
snapshot provenance agree. An unmatched legacy reservation is released without
deleting its deck, generated provenance, or operational history. Existing
reviewed/graduated decks and Known-vocabulary rows remain historical truth.

Structural migrations and data backfills remain separate and follow
[ADR 0038](0038-schema-change-governance.md). The baseline migration is not
edited. Recovery after a failed backfill is retrying the idempotent step, not
silently inventing a vocabulary transition.

## Alternatives considered

- **Keep deck review as the graduation event.** Rejected: the learner's explicit
  acceptance of a completed Primary Goal is the product event; deck readiness and
  review are optional artifact concerns, not a second learner plan.
- **Keep Book- or deck-owned reservation state.** Rejected: reservation follows
  the active Goal and its immutable snapshot, so clearing or changing a Goal has
  one unambiguous consequence.
- **Continue excluding generated vocabulary.** Rejected: generated provenance
  records assignment, not knowledge. A later Goal must be able to model an
  identity again unless it is Known or currently Reserved.
- **Keep an advisory vocabulary-efficient order.** Rejected: it competes with
  learner direction. Sequential on-arrival forecast explains the consequences of
  the learner's own order without recommending a different route.
- **Treat unavailable predecessors as zero contribution.** Rejected: zero is a
  fabricated observation. Downstream values are useful only when labeled lower
  bounds.

## Consequences

- The active learner state is Reading Journey membership/order, one per-language
  Primary Goal, the Goal's frozen snapshot, durable reading completion, and
  Known vocabulary; deck jobs remain operational artifact state.
- Goal choice and completion become the only learner-facing reservation and
  vocabulary-transition actions. Separate study, review-confirmation, and
  release actions are retired from the active path.
- Forecasts explain sequence effects while preserving learner order and exact
  vocabulary set semantics.
- Existing historical artifacts remain auditable, but they no longer create
  current learner-state exclusions or retroactive knowledge.
- Schema, persistence, handler, and browser work must implement these contracts
  under the follow-on issues without adding compatibility layers for the retired
  unshipped learner flows.

## Non-goals

- Objective per-card mastery, spaced-repetition tracking, or AnkiConnect.
- Automatic selection or reordering of a next Goal.
- A composite difficulty, literary-value, readiness, or recommendation score.
- Persisting forecast percentages or a computed alternative order.
- Changing the frequency-floor value or exposing it as learner configuration.
- Deleting historical decks, generated provenance, graduation provenance, or
  Known-vocabulary rows.

## Related

- [Issue #1051](https://github.com/justin-hayes/mouseion/issues/1051) and parent
  [#1050](https://github.com/justin-hayes/mouseion/issues/1050).
- [ADR 0048](0048-frequency-floor-deck-selection.md) for recurring-vocabulary
  selection.
- [ADR 0049](0049-reading-intent-triggers-analysis.md) for Journey-triggered
  analysis.
- [ADR 0050](0050-active-study-language.md) and
  [ADR 0051](0051-reading-journeys-and-goals-per-language.md) for language scope.
- [ADR 0070](0070-migration-and-documentation-reboot.md) for documentation and
  migration governance.
