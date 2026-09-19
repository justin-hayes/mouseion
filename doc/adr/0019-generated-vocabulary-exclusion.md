# ADR 0019: Explicit generated-vocabulary exclusion policy

Status: **Superseded by ADR 0072** · Date: 2026-08-23 · Author: Justin + Hermes

Amends **ADR 0017** (Replace frequency-based ranking with coverage-based selection).

## Context

The deck algorithm selects unknown lemmas from the eligible unknown pool after
learner-state exclusions (coverage-based selection under ADR 0017, amended to a
frequency floor by ADR 0048). The learner has two distinct reasons a word
should not be selected:

1. The learner has explicitly recorded it in `known_vocabulary`.
2. Mouseion has already exported it in a generated deck for that learner.

The second category is not equivalent to mastery. Exporting a word assigns it
for study; it does not prove that the learner knows it. The previous
implementation represented exported words indirectly through
`vocabulary_states.state = 'generated'`, which conflated deck history with the
vocabulary lifecycle and did not retain clear book/deck provenance.

## Decision

Maintain an explicit owner-scoped `generated_vocabulary` record keyed by:

- owner
- language
- canonical lemma
- UPOS

Each record retains the first generated deck, source book when available, and
generation timestamp. Recording the generated identity is atomic with card
creation and idempotent for repeated exports.

Coverage selection excludes both:

- explicit known vocabulary; and
- generated vocabulary whose source book differs from the current book.

These exclusions occur before calculating the coverage denominator. Words
excluded by either policy are treated as already covered for the new deck.
Words exported from the same book remain safely re-exportable, with card
uniqueness preventing duplicate cards.

Exporting a word does **not** insert it into `known_vocabulary`. Future product
work may provide an explicit learner action to promote an assigned/generated
word to known vocabulary.

## Consequences

- Deck history becomes explicit, queryable, owner-scoped, and auditable.
- A word exported in one book will not be selected for a different book's deck
  for the same learner.
- The remaining selection rule (coverage-based under ADR 0017, amended to a
  frequency floor by ADR 0048) applies to the eligible unknown pool after
  known/generated exclusions.
- Existing `vocabulary_states.state = 'generated'` values may be retained
  temporarily for compatibility, but they are no longer the authoritative
  cross-book exclusion mechanism.
- Existing physical frequency/ranking tables and columns may remain until a
  later non-destructive cleanup; they are unrelated to this exclusion policy.

*Provenance note: the principle that "generated is not known" is retained; the
single justified graduation transition from a campaign snapshot into
`known_vocabulary` is defined by ([ADR 0036](0036-primary-goal-justified-graduation.md)).*
