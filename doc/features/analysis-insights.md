# Analysis Insights

Status: Implemented · Date: 2026-08-24

## Problem

After a book is analyzed, a learner needs more than an analyzed status or a deck button. They need evidence for deciding whether to read the book now, prepare vocabulary first, or choose another book.

## Goals

- Show how much of the book is covered by the learner's known vocabulary.
- Show the vocabulary investment required to reach 95%, 97%, and 99% token coverage.
- Explain the difference between current known, active-campaign projected,
  unknown, legacy generated, and eligible vocabulary.
- Surface useful text-profile and analysis-quality signals.
- Keep every metric explainable and reproducible from the analyzed book and learner vocabulary.

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

The UI should also show distinct lemma counts and occurrence counts because
lemma coverage and token coverage answer different questions. Percentages use
the integer occurrence counts; presentation may round the resulting ratio, but
selection never uses a rounded percentage.

### Threshold requirements

Threshold investment uses the full analyzable-token denominator from current
coverage. Remove explicitly known identities from the unknown-to-learn list, but
include previously generated identities: generated means assigned for study, not
known. For target `T`, additional vocabulary to learn is the smallest
frequency-ordered prefix of all currently unknown lemma+UPOS identities which,
when added to explicitly known occurrences, reaches `T` percent of all
analyzable tokens. If current coverage already reaches the target, the required
lemma count is zero.

Candidates are ordered by descending book-local occurrence count. Equal counts
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
reserved vocabulary according to the learning-campaign lifecycle in ADR 0027.

Initial targets:

- 95%
- 97%
- 99%

### Vocabulary categories

The UI must distinguish:

- current known vocabulary, including completed-campaign graduates;
- potential coverage after the active campaign graduates;
- unknown vocabulary;
- unattached legacy generated vocabulary during the compatibility transition;
- vocabulary eligible for a new deck.

Generated or active-campaign vocabulary is not silently reported as known.
Queued books show current-known coverage and future-book coverage after the
active campaign graduates. Both values are calculated on demand, so completing
or abandoning the active campaign changes the queue without a persisted
coverage or mastery snapshot.

## Initial presentation

An analyzed book should eventually expose:

- analyzable token count;
- distinct lemma count;
- known-token coverage;
- unknown-token count;
- known and unknown distinct lemmas;
- additional lemmas for 95%, 97%, and 99%;
- top unknown lemmas by occurrence count;
- warnings about incomplete or low-quality analysis.

The lexical profile also shows the five highest-occurrence deck-eligible unknown
lemmas, the share of eligible unknown occurrences concentrated in the top ten,
and projected overall token coverage after learning the top 10, 25, or 50.
Active-campaign reservations and unattached legacy generated vocabulary remain
excluded from these learn-next projections and are not counted as known.
Abandoned campaign vocabulary returns to the eligible pool. Projection percentages use the full analyzable-token
denominator and never exceed 100%.

The structural profile shows the analyzer-provided sentence count, median and
90th-percentile sentence length, and the share of sentences longer than 35
tokens. Sentence length counts every normalized token, including punctuation.
The median averages the two middle values for an even number of sentences; p90
uses the nearest-rank value. These signals remain separate and descriptive.

The profile also shows the analyzable-token count alongside the total normalized
token count.
An analysis-quality region reports only gaps that can be reproduced directly
from persisted analyzer output: no sentences, analyzer-provided sentences with
no tokens, or normalized tokens with no vocabulary-analyzable tokens. Each
warning explains which metrics are affected; the UI does not infer a general
quality grade from these checks. Legacy analyses do not have reproducible
coverage or sentence aggregates and must be rerun to expose analysis insights.

## Difficulty dimensions

Do not collapse difficulty into one unexplained number. Keep separate dimensions:

- lexical coverage and unknown-word concentration;
- sentence length and structural complexity;
- named-entity, quotation, and editorial-material density;
- extraction and analysis quality.

The structural metric contract is recorded in [ADR 0026](../adr/0026-structural-text-profile.md).

## Documentation and delivery

The repository feature document is the product source of truth. Stable metric decisions may be recorded in a dedicated ADR. GitHub issues track implementation slices and PRs provide delivery/verification history. No GitHub Project or GitHub Milestone is required for this feature. The Obsidian vault records the broader milestone and links back to this document and issues.

The lexical contract is recorded in [ADR 0025](../adr/0025-analysis-coverage-threshold-metrics.md),
and the structural contract in [ADR 0026](../adr/0026-structural-text-profile.md).

Thresholds, learner coverage, and projections are computed on demand from the
persisted corpus statistics and current owner-scoped vocabulary state. This
keeps the learner-specific values current after known-vocabulary or generated-
deck changes without persisting derived mastery claims.
