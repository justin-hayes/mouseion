# ADR 0027: Single-active learning campaigns and vocabulary graduation

Status: **Accepted (completion and graduation semantics superseded by ADR 0035)** · Date: 2026-08-24 · Author: Justin + Hermes

Supersedes the future-selection semantics of [ADR 0019](0019-generated-vocabulary-exclusion.md) while retaining its distinction between explicit knowledge, generated provenance, and learner ownership. The Campaign object and its reservations/graduation remain valid internal state; ADR 0035 supersedes the learner-facing completion and vocabulary-graduation transitions (a deck-independent Primary Goal, reading-only operation, and a single graduated path keyed on snapshotted, provenance-linked identities plus justified study confirmation).

## Context

Mouseion currently records generated vocabulary as a permanent cross-book exclusion. That treats deck assignment as a durable selection state, even when a learner never studies or completes the associated book. It also makes it difficult to represent the learner's actual workflow: one active book/deck at a time, a queue of future books, and vocabulary becoming known after the active reading/study campaign is complete.

Generated is not the same as known. A deck export proves only that Mouseion assigned a lemma for study. It does not prove recognition or mastery.

## Decision

Introduce a learner-owned **learning campaign** that ties together one source book and one prepared deck. A learner may have at most one active campaign. Other analyzed books may be queued for later.

A campaign has these independent progress facts:

- book reading status: queued, reading, finished, or abandoned;
- deck study status: queued, studying, reviewed, or abandoned;
- derived campaign status: queued, active, complete, or abandoned;
- completion timestamps and vocabulary-graduation timestamp.

A campaign is complete when both conditions hold:

```text
book_status = finished
AND
deck_status = reviewed
```

The initial product may let the learner mark these facts manually. Mouseion cannot infer Anki review history from a one-way `.apkg` export. Future Anki review-log or AnkiConnect integration may replace manual confirmation.

### Vocabulary lifecycle

```text
unknown → actively assigned → graduated/known
                         ↘ abandoned → unknown
```

- **Known vocabulary** includes explicit user imports/marks and vocabulary graduated from completed campaigns.
- **Active campaign vocabulary** is reserved for the active workflow but is not counted as known.
- **Generated vocabulary** remains immutable provenance: what Mouseion assigned, for which deck/book, and when.
- **Abandoned campaign vocabulary** becomes eligible again unless independently marked known.
- **Graduated vocabulary** is permanently known and remains linked to its campaign provenance.

Completing a campaign promotes its assigned vocabulary into the learner's known vocabulary and records the graduation source, campaign, book, deck, and timestamp. This is a learner-state transition, not a rewrite or deletion of generated history.

### Coverage semantics

Book coverage reports explicit current knowledge, including graduated campaign vocabulary. It does not count merely generated or actively assigned vocabulary as known.

The product may additionally show a clearly labeled projection:

```text
Potential coverage if the active campaign is completed
```

That projection is not current knowledge.

Future-book selection uses:

- known vocabulary as excluded mastery;
- active campaign vocabulary as temporarily reserved;
- abandoned vocabulary as eligible again;
- no permanent exclusion solely because a word was once generated.

The one-active-campaign rule prevents multiple simultaneous decks from reserving overlapping vocabulary and makes queue recalculation deterministic.

## Consequences

### Positive

- The data model reflects the learner's real book-plus-deck workflow.
- Abandoned decks do not create permanent blind spots.
- Future-book coverage can be recalculated after campaign completion.
- Generated provenance and known status remain distinct and explainable.
- A queue can be planned without pretending queued vocabulary is mastered.

### Costs

- A learning-campaign persistence model and lifecycle UI are required.
- Existing generated-vocabulary exclusions need a compatibility transition.
- Manual book/deck completion controls are needed before review integration exists.
- Coverage pages need current versus projected labels.

## Non-goals

- No mastery model or spaced-repetition grade is introduced.
- No Anki synchronization is required for the first implementation.
- No simultaneous multi-deck learning is supported.
- No deletion of generated-vocabulary provenance occurs.

## Migration and compatibility

Existing generated-vocabulary rows remain historical provenance. Existing rows must be treated conservatively during migration: until their campaign relationship is known, they should not be silently converted into explicit known vocabulary. A compatibility strategy may temporarily retain their old exclusion behavior while new campaign rows use active/graduated/abandoned state. The implementation milestone must define and test the transition before changing selection behavior.

## References

- [ADR 0019: Generated-vocabulary exclusion](0019-generated-vocabulary-exclusion.md)
- [ADR 0022: Prepared decks](0022-prepared-decks.md)
- [Analysis Insights feature](../features/analysis-insights.md)
