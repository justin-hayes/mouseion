# ADR 0048: Frequency-floor deck selection

Status: **Accepted** · Date: 2026-09-07 · Author: Justin (via OpenChamber)

Amends **ADR 0017** (Replace frequency-based ranking with coverage-based
selection) and the deck-generation clause of **ADR 0025** (Analysis coverage and
threshold metric contract).

## Context

The deck pool has selected the smallest prefix of eligible unknown lemmas,
ordered by book-local occurrence count, whose cumulative occurrences reach 97%
of the eligible unknown pool. On real analyzed corpora that prefix contains every
lemma appearing three or more times and then extends into the long tail of one-
and two-occurrence words: 48-80% of current deck cards are for words that appear
once or twice. Those rare-word cards are numerous, low-yield study targets that
pad the deck without carrying the learner through the frequent vocabulary.

## Decision

Replace the coverage-prefix selection with a **frequency floor**. The deck
selects every eligible unknown lemma appearing at least `N` times in the
analyzed book, where `N` is a parameter with a default of `3`. The deck makes no
coverage claim.

The frequency floor is applied at deck-pool time over persisted occurrence
counts. It changes no analysis-time filters: proper names, the POS allowlist,
Known and active Goal-derived Reserved eligibility (ADR 0072), and the sentence
quality gates are unchanged. Generated history is provenance, not an exclusion.
Cards remain ordered by first encounter in the text.

## Rationale

- A frequency floor cuts the 1-2 occurrence tail directly, which was the source
  of deck bloat; coverage becomes a derived property of the resulting set, not a
  target it must reach.
- A book with no unknown lemma appearing at least `N` times yields an empty deck
  rather than silently re-adding rare words.
- Applied at deck-pool time, the change affects already-analyzed books without
  re-analysis and leaves the analyzable-token denominator and whole-book insight
  thresholds untouched.

## Consequences

- Decks no longer guarantee 97% coverage of the eligible unknown pool. On
  current corpora the selected pool covers roughly 91% of the unknown tokens for
  a book-length work and roughly one-third to one-half for shorter works.
- The 95/97/99 insight thresholds on the book page (ADR 0025) are unchanged:
  they are whole-book planning metrics, not deck-selection targets.
- The default `N` is a named constant in the deck-pool selection, not yet
  surfaced for customization.
- The 97% deck-selection clause of ADR 0025 is amended accordingly.

## Alternatives considered

- **Keep the 97% prefix and add the floor as a cap.** In practice identical to
  this decision for current corpora, because the prefix already contains every
  ≥3 word. Rejected for keeping a coverage claim the selection no longer honors.
- **Coverage floor layered under the frequency floor.** Reintroduces the 1-2
  occurrence tail for short books. Rejected.
- **Raise the analysis-time minimum-occurrence filter to 3.** Changes the
  analyzable-token denominator, silently rewrites known-coverage and threshold
  metrics, and requires re-analysis. Rejected.

## References

- [ADR 0017: Replace frequency-based ranking with coverage-based selection](0017-coverage-based-selection.md)
- [ADR 0025: Analysis coverage and threshold metric contract](0025-analysis-coverage-threshold-metrics.md)
- [ADR 0072: Goal-owned vocabulary snapshots and sequential Reading Journey forecast](0072-goal-owned-vocabulary-and-journey-forecast.md)
- [Terminology](../design/terminology.md)
