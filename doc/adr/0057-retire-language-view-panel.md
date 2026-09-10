# ADR 0057: Retire the Language view panel

Status: Accepted · Date: 2026-09-10 · Supersedes: ADR 0042

## Context

ADR 0042 proposed a derived, evidence-only Language view panel within My Books.
The panel would have aggregated current analyses and known vocabulary across a
learner's Books in one language. The implementation work that followed has been
retired: per-Book evidence remains on My Books rows, while the Journey entry
already presents the current analysis evidence and the decisions that depend on
it.

The panel would duplicate those existing evidence surfaces without owning a
Book, analysis, Journey, or learner action. Keeping it in the product contract
would also leave the retired language-corpus read model and a second aggregate
place to interpret evidence as apparent current architecture.

## Decision

- The My Books Language view panel is retired. It has no route, read model, or
  learner-facing screen contract.
- My Books remains the broad bibliographic collection. Its per-Book rows present
  the Book's evidence state and available collection or Journey actions.
- The Journey entry remains the current completed-analysis surface. Its
  per-Book insights and Journey-level comparison provide evidence where the
  learner can act on it.
- Mouseion does not persist or derive a replacement cross-Book language
  aggregate. The internal `Corpus` and `normalized_corpus_artifacts` terms keep
  their analysis-artifact meanings.
- ADR 0042 and the Language Corpus View feature document remain available as
  historical context and must not be read as current product requirements.

## Alternatives considered

- **Keep the panel within My Books.** Rejected: it duplicates evidence already
  carried by the Book and Journey surfaces and adds another interpretation layer
  without a distinct learner action.
- **Promote the panel to a primary destination.** Rejected: the canonical
  architecture is organized around My Books, Reading Journey, and Vocabulary;
  the panel does not justify a fourth destination.
- **Persist a language aggregate for later use.** Rejected: no current learner
  workflow requires a new aggregate lifecycle, and persisted aggregate evidence
  would introduce staleness without improving an available decision.

## Consequences

- The learner has one clear place for collection evidence and one clear place for
  current analysis decisions, without a duplicate language dashboard.
- Known vocabulary and current analyses no longer imply an additional language
  surface or read model.
- The Language Corpus View proposal and ADR 0042 remain useful for historical
  design context, but future work must not treat them as an implementation
  target without a new accepted decision.

## Related

- [ADR 0042: Derive a per-language corpus view without a persisted corpus object](0042-derived-language-corpus-view.md)
- [ADR 0045: Book detail is addressed by owner-scoped Book ID, with source IDs resolving in place](0045-book-detail-book-id.md)
- [ADR 0050: The app works in one active study language at a time](0050-active-study-language.md)
- [Language Corpus View feature](../features/language-corpus-view.md)
- [Information architecture](../design/information-architecture.md)
