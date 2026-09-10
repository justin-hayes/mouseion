# Analysis Insights

Status: Implemented; learner-facing simplification proposed in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) · Date: 2026-08-24 ·
Updated: 2026-09-02

## Problem

After a book is analyzed, a learner needs more than an analyzed status or a deck button. They need evidence for deciding whether to read the book now, prepare vocabulary first, or choose another book.

## Goals

- Show how much of the book is covered by the learner's known vocabulary.
- Show the vocabulary investment required to reach 95%, 97%, and 99% token coverage.
- Show the highest-impact unknown vocabulary without presenting every internal
  vocabulary-state category.
- Surface only concrete, reproducible analysis-quality gaps as warnings.
- Keep every metric explainable and reproducible from the resulting corpus scope and learner vocabulary.

## Non-goals

- A single opaque reading-difficulty score.
- Treating previously generated vocabulary as mastered vocabulary.
- Introducing corpus-scale core-vocabulary resources in this feature.
- Estimating calendar time to mastery without an explicit learner-rate model.
- Presenting heuristic structural signals as authoritative proficiency levels.

## Metric contract

### Vocabulary coverage

Primary coverage is token-weighted:

```text
known analyzable token occurrences / total analyzable token occurrences
```

An analyzable token occurrence is an occurrence retained by the existing NLP and
selection filters: punctuation, proper names, and stop words are outside the
metric. The denominator is the sum of retained occurrence counts before learner
vocabulary state is applied. Explicitly known occurrences and vocabulary
graduated by completed campaigns contribute to the numerator. Active-campaign
vocabulary remains a separate projection and is not counted as known.
Abandoned campaign vocabulary is unknown and eligible again.
Graduation follows the single justified transition in
[ADR 0036](../adr/0036-primary-goal-justified-graduation.md): only
`learning_campaign_vocabulary` identities atomically linked to generated
provenance and confirmed by deck review graduate; reading-finished alone
graduates nothing.

The underlying calculation retains distinct lemma counts and occurrence counts
because lemma coverage and token coverage answer different questions. The
learner-facing Journey entry presents only current known coverage from this
coverage-stat family. Percentages use the integer occurrence counts;
presentation may round the resulting ratio, but selection never uses a rounded
percentage.

### Threshold requirements

Threshold investment uses the resulting corpus scope's full analyzable-token denominator from current
coverage. Remove explicitly known identities from the unknown-to-learn list, but
include previously generated identities: generated means assigned for study, not
known. For target `T`, additional vocabulary to learn is the smallest
frequency-ordered prefix of all currently unknown lemma+UPOS identities which,
when added to explicitly known occurrences, reaches `T` percent of all
analyzable tokens. If current coverage already reaches the target, the required
lemma count is zero.

Candidates are ordered by descending analyzed-scope occurrence count. Equal counts
are ordered lexicographically by `(language, canonical lemma, UPOS)`. The
threshold comparison uses exact integer arithmetic:

```text
(known occurrences + selected unknown occurrences) * 100
    >= analyzable occurrences * T
```

An explicit known-vocabulary entry with no UPOS is a lemma wildcard and covers
every UPOS for that language. An entry with a UPOS covers only the matching
lemma+UPOS identity. Previously generated identities may be annotated as
already assigned, but they remain part of the learner's unknown-to-learn pool.
The separate deck-generation calculation continues to exclude active or
reserved vocabulary according to the learning-campaign lifecycle in
[ADR 0027](../adr/0027-learning-campaigns.md), as governed by
[ADR 0036](../adr/0036-primary-goal-justified-graduation.md).

Initial targets:

- 95%
- 97%
- 99%

### Vocabulary categories

The calculation and operational evidence distinguish:

- current known vocabulary, including completed-campaign graduates;
- potential coverage after the active campaign graduates;
- unknown vocabulary;
- unattached legacy generated vocabulary during the compatibility transition;
- vocabulary eligible for a new deck.

Generated or active-campaign vocabulary is not silently reported as known. The
Journey entry leads only with current known coverage; vocabulary investment and top
unknowns apply the distinct categories internally. Books later in Reading
Journey may show current-known coverage and future-book coverage after the
active campaign graduates. Both values are calculated on demand, so completing
or abandoning the active campaign changes Journey evidence without a persisted
coverage or mastery snapshot.

## Initial presentation

The Journey entry at `/journey/{bookID}` shows one current analysis for a
Journey member. Its
presentation keeps:

- **Current known coverage** as the headline and premier metric, with a
  one-line qualifier such as “of the analyzed units”;
- **Vocabulary investment** / **Additional vocabulary** for the 95%, 97%, and
  99% targets;
- **Highest-impact unknown vocabulary**, led by the unknown lemmas with the
  greatest contribution;
- deck preparation; and
- a single compact analysis-quality note only when persisted analyzer output
  exposes a concrete warning.

The learner-facing presentation removes analyzed-scope detail, text profile,
projected token coverage after the top 10/25/50 lemmas, the detailed
coverage-stat list other than current known coverage, and analysis history.
The underlying metric contracts, persisted aggregates, and operational audit
records remain intact.

Quality warnings are limited to gaps reproducible directly from persisted
analyzer output: no sentences, analyzer-provided sentences with no tokens, or
normalized tokens with no vocabulary-analyzable tokens. Each rendered warning
explains which retained metric or action is affected; the UI does not infer a
general quality grade. When none of these gaps exists, the page renders no
analysis-quality note. Legacy analyses without reproducible coverage or
sentence aggregates must be rerun to become the current analysis.

## Difficulty dimensions

Do not collapse difficulty into one unexplained number. Persist these dimensions
separately even though the current learner-facing analysis surface does not
display a text profile:

- lexical coverage and unknown-word concentration;
- sentence length and structural complexity;
- named-entity, quotation, and editorial-material density;
- extraction and analysis quality.

The structural metric contract is recorded in [ADR 0026](../adr/0026-structural-text-profile.md).

## Documentation and delivery

The repository feature document is the product source of truth. Stable metric decisions may be recorded in a dedicated ADR. GitHub issues track implementation slices and PRs provide delivery/verification history. No GitHub Project or GitHub Milestone is required for this feature. The Obsidian vault records the broader milestone and links back to this document and issues.

The lexical contract is recorded in [ADR 0025](../adr/0025-analysis-coverage-threshold-metrics.md),
and the structural contract in [ADR 0026](../adr/0026-structural-text-profile.md).

## Cross-book projections and advisory Journey ordering

Per-book analysis insights are the technical seed for the Reading Journey route
comparison. The accepted cross-book contract is recorded in
[ADR 0037](../adr/0037-cross-book-projection-advisory-ordering.md): the
vocabulary-efficient alternative to the learner's canonical Journey order
optimizes exactly one named lexical property — current known-token coverage — using
a reproducible objective over the learner-selected comparable books and
fixed-order constraints. It reuses ADR 0025's per-book coverage, denominator, and
integer comparison without introducing a new aggregate or composite score. Current
and conditional projected coverage states remain distinct, incomparable books stay
at the learner's position without a fabricated rank, and the alternative is
computed on demand and never persisted as stale truth. The learner's canonical
order is always the active order; the alternative is advisory comparison evidence
only and never auto-reorders, auto-selects a Primary Goal, or implies literary or
difficulty judgment.

Thresholds, learner coverage, and projections are computed on demand from the
persisted corpus statistics and current owner-scoped vocabulary state. This
keeps the learner-specific values current after known-vocabulary or generated-
deck changes without persisting derived mastery claims.

## EPUB analysis and reanalysis

The current analysis processes the entire acquired EPUB. Its coverage,
thresholds, and top unknowns use the resulting corpus; the one-line coverage
qualifier states that the headline applies to the analyzed book without
exposing a separate scope-detail section.

Deck preparation starts on the Journey entry but remains bound internally to the
exact immutable completed analysis that owns the corpus. It is not available
for queued, running, failed, cancelled, or legacy-only analysis state.

Reanalysis explicitly submits a new whole-book run for the current source
revision. When the run completes, it replaces the book's
single current learner-facing analysis. Earlier immutable analyses and corpora
remain owner- and source-material-scoped operational audit records available
through `/jobs`, not learner-facing result history. Internal source and
extracted-unit provenance remains available for audit and compatibility without
exposing a learner-facing scope-confirmation step.
