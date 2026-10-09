# Analysis Insights

Journey and Primary Goal projections in this feature are historical. Current
Reading presents current coverage bands only; the sequential forecast is retired
as described in the [Reading workflow](reading-workflow.md).

Status: Implemented · Date: 2026-08-24 · Updated: 2026-09-15

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
vocabulary state is applied. Explicitly imported occurrences and vocabulary
accepted by completed Primary Goals contribute to the numerator. Reserved
vocabulary remains a separate projection and is not counted as Known. Generated
vocabulary is provenance, not an exclusion or knowledge claim. The transition
follows [ADR 0072](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md):
the active Goal's frozen snapshot is accepted atomically on completion, with set
semantics and no claim of verified mastery. Deck readiness or review is not a
precondition.

The underlying calculation retains distinct lemma counts and occurrence counts
because lemma coverage and token coverage answer different questions. The
learner-facing Reading Journey presents Book identity and current evidence from this
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
The separate deck-generation and Goal-snapshot calculation excludes only
current Known and active Goal-derived Reserved vocabulary in the relevant study
language, as governed by ADR 0072. Historical generated rows do not exclude an
identity from a later selection.

Initial targets:

- 95%
- 97%
- 99%

### Vocabulary categories

The calculation and operational evidence distinguish:

- current Known vocabulary, including completed-Goal additions;
- potential coverage after the active Goal snapshot is accepted;
- unknown vocabulary;
- generated vocabulary as immutable historical provenance;
- vocabulary eligible for a new deck.

Generated or reserved vocabulary is not silently reported as Known. Reading
Journey uses the distinct categories for its current evidence and forecast; the
retired vocabulary-investment and top-unknown presentation remains available
only to internal analysis services. The Journey overview additionally
shows current, after-Goal, and on-arrival coverage from the learner's own order.
All values are calculated on demand, so Goal changes, completion, reordering, or
evidence changes do not leave a persisted coverage or mastery snapshot.

## Initial presentation

The former `/journey/{bookID}` URL is a compatibility bookmark that redirects a
reachable Journey member to the Book's anchor in `/journey`; it never renders a
generic Book or analysis-result page. Reading Journey owns Book identity,
relationship, current evidence, forecast, and recovery. Focused preparation is
opened separately at `/journey/books/{bookID}/deck/preparations/new`.

The learner-facing presentation removes vocabulary investment, highest-impact
unknown vocabulary, analyzed-scope detail, text profile, projected token
coverage, the detailed coverage-stat list, and analysis history. The underlying
metric contracts, persisted aggregates, thresholds, top-unknown data, and
operational audit records remain intact.

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

The repository feature document is the product source of truth. Stable metric decisions may be recorded in a dedicated ADR. GitHub issues track implementation slices and PRs provide delivery/verification history. No GitHub Project or GitHub Milestone is required for this feature.

The lexical contract is recorded in [ADR 0025](../adr/0025-analysis-coverage-threshold-metrics.md),
and the structural contract in [ADR 0026](../adr/0026-structural-text-profile.md).

## Historical Journey forecast (retired)

The sequential Journey forecast described by [ADR 0072](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
was retired with Journey ordering. It is not a current learner-facing screen or
metric. The current Reading chooser groups To Read candidates by current
coverage evidence only; it does not show after-Goal/on-arrival projections or
rank candidates. See the shipped [Reading workflow](reading-workflow.md).

Thresholds and learner coverage are computed on demand from the
persisted corpus statistics and current owner-scoped vocabulary state. This
keeps the learner-specific values current after known-vocabulary or generated-
deck changes without persisting derived mastery claims.

## EPUB analysis and reanalysis

The current analysis processes the Book's identified main text when declared
EPUB structure provides one, and the entire acquired EPUB otherwise
([ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md)). Its
coverage, thresholds, and top unknowns use the resulting corpus; the one-line
coverage qualifier states that the headline applies to the analyzed book without
exposing a separate scope-detail section.

Deck preparation starts in the focused task but remains bound internally to the
exact immutable completed analysis that owns the corpus. It is not available
for queued, running, failed, cancelled, or legacy-only analysis state.

Reanalysis explicitly submits a new run for the current source revision; it is
the point at which a Book with an older whole-book result adopts main-text
selection. When the run completes, it replaces the book's
single current learner-facing analysis. Earlier immutable analyses and corpora
remain owner- and source-material-scoped operational audit records available
through `/jobs`, not learner-facing result history. Internal source and
extracted-unit provenance remains available for audit and compatibility without
exposing a learner-facing scope-confirmation step.
