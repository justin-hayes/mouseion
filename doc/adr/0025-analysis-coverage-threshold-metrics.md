# ADR 0025: Analysis coverage and threshold metric contract

Status: **Accepted; graduation-trigger semantics amended by ADR 0027 and ADR 0036** · Date: 2026-08-24 · Author: Justin + Codex

Clarifies **ADR 0017** (Replace frequency-based ranking with coverage-based
selection) and **ADR 0019** (Explicit generated-vocabulary exclusion policy).

## Context

Analysis insights need reproducible answers to two related but different
questions: how much of a book the learner explicitly knows, and how many new
vocabulary identities Mouseion would select at a target threshold. The existing
deck path already selects 97% of the unknown-token pool after known and generated
exclusions. The metrics contract must preserve that behavior while supporting
95%, 97%, and 99% projections and without reporting generated vocabulary as
mastered.

## Decision

An **analyzable token occurrence** is an occurrence retained by the current NLP
and selection filters. Punctuation, proper names, and stop words are excluded.
The total analyzable-token denominator is the sum of those retained occurrence
counts before applying learner vocabulary state.

**Known-token coverage** is:

```text
explicitly known analyzable occurrences / total analyzable occurrences
```

Explicit known vocabulary is language-scoped. A `(lemma, UPOS)` entry matches
only that identity; an entry whose UPOS is empty is a lemma wildcard and matches
all UPOS for that lemma. Generated vocabulary is never counted as explicitly
known.

**Threshold investment** uses the same full analyzable-token denominator as
known-token coverage. Remove explicitly known identities and owner-scoped
generated identities from other books from the learn-next candidates. Generated
records with unknown provenance are excluded conservatively; records first
generated for the current book remain eligible for idempotent repeat exports.
The remaining occurrences are the eligible unknown-token pool.

For target `T`, order eligible identities by descending book-local occurrence
count, breaking equal-count ties lexicographically by `(language, canonical
lemma, UPOS)`. Select the shortest prefix for which:

```text
(explicitly known occurrences + selected occurrences) * 100
    >= total analyzable occurrences * T
```

The result is an identity count and occurrence count, not a claim that the
learner knows those words. Integer comparison is authoritative; rounded display
percentages do not affect selection. If all eligible identities cannot meet the
comparison because generated or otherwise excluded occurrences remain unknown,
the target is explicitly unreachable and no lemma count is presented. Initial
targets are 95, 97, and 99. The deck-generation path continues to select 97% of
its eligible unknown pool under ADR 0019; its count can therefore differ from an
insight threshold with the same numeric label.

## Consequences

- Current coverage, projections, and insight thresholds share the full
  analyzable-token denominator.
- Known, generated, eligible unknown, and selected vocabulary remain distinct
  categories, so generated cards do not inflate mastery.
- Equal-frequency corpora produce stable results independent of database row
  order.
- The 97% deck output is unchanged and is not presented as the 97% whole-book
  insight threshold.
- Each newly analyzed owner-scoped corpus persists total analyzable occurrences
  and distinct lemma+UPOS identity count. Legacy corpora leave both values
  absent because existing artifacts cannot reproduce the named-entity filter.
- Known and unknown counts are derived from current owner vocabulary state, not
  persisted as immutable analysis metadata, so vocabulary updates cannot make
  them stale.
- UI representation and threshold projections remain deferred to the follow-up
  implementation issues.

## Alternatives considered

- **Use the eligible unknown pool as the insight-threshold denominator.**
  Rejected because it labels a pool-coverage investment as whole-book coverage
  and can hide the fact that excluded unknown tokens make a target unreachable.
- **Count generated vocabulary as known.** Rejected because assignment for study
  is not evidence of mastery.
- **Break ties by first encounter.** Rejected for selection because the existing
  contract uses identity ordering; first encounter remains the final deck order.
- **Use floating-point percentages.** Rejected because exact integer arithmetic
  is simpler and avoids rounding-dependent boundary results.

## References

- [Analysis Insights feature](../features/analysis-insights.md)
- [ADR 0017: Replace frequency-based ranking with coverage-based selection](0017-coverage-based-selection.md)
- [ADR 0019: Explicit generated-vocabulary exclusion policy](0019-generated-vocabulary-exclusion.md)
- [ADR 0036: Deck-independent Primary Goal and single justified vocabulary-graduation transition](0036-primary-goal-justified-graduation.md) — defines the single justified path by which identity graduates into the `known_vocabulary` that feeds this metric's numerator; the denominator, integer comparison, and threshold selection here are unchanged.
