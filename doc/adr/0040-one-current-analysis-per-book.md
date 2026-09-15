# ADR 0040: One current analysis per book

Status: **Accepted** · Date: 2026-09-02 · Author: Justin + Codex · Learner lifecycle wording reconciled by [ADR 0054](0054-retire-standalone-analysis-action.md)

## Context

Mouseion currently gives a completed analysis two learner-facing surfaces: the
book page summarizes lifecycle and analysis history, while a run-specific page
at `/books/{id}/analyses/{runID}` presents detailed insights, provenance, and
deck preparation. That split was justified when learners could choose among
multiple immutable analyses for one book and needed to understand which exact
scope and run they were viewing.

For the ordinary reading decision, the split now repeats the same coverage,
vocabulary-investment, and unknown-vocabulary evidence while making the learner
navigate through analysis identity and history. The book, not an analysis run,
is the learner's object of attention. Immutable scope, analysis, corpus, and
prepared-deck identities remain necessary for reproducibility and operations,
but they do not require parallel learner-facing result pages.

## Decision

Each book has at most one learner-facing **current analysis**, derived from an
immutable confirmed scope. A newly completed reanalysis replaces the book's
current analysis rather than adding a second learner-facing result. Earlier
analyses remain immutable backend and operational audit records; they are
reachable through `GET /jobs` and applicable job detail, but are not listed on
the book page or exposed through another learner result surface.

The book page at `/books/{id}` and the Journey entry for a member share one
learner-facing current-analysis presentation. My Books and Reading Journey own
the lifecycle actions for acquisition and analysis; the page presents one
coherent set of decision evidence when a current analysis is available:

- **Current known coverage** as the headline and premier metric, with a
  one-line qualifier such as “of the analyzed units” rather than a separate
  scope section;
- **Vocabulary investment** / **Additional vocabulary** thresholds;
- **Highest-impact unknown vocabulary** by contribution;
- the **Deck preparation** action; and
- one compact analysis-quality note only when persisted analyzer output exposes
  a concrete analyzer or data-integrity gap, such as no analyzable tokens,
  empty sentences, or another quality warning. A clean analysis renders no
  quality note.

The learner-facing page removes **Analyzed scope**, **Text profile**,
**Projected token coverage**, the detailed coverage-stat list other than
current known coverage, and **Analysis history**, including their subordinate
content. The persisted inputs and metrics governed by ADRs 0025 and 0026 remain
available to application logic and operations; this decision narrows their
learner-facing presentation rather than deleting the underlying evidence.

`GET /books/{id}/analyses/{runID}` becomes a thin compatibility redirect to the
current Book page or the member's Journey entry. It preserves existing deep
links, completed-job **View analysis result** links, and deck-preparation
references without remaining a separate full-insight surface. Its former
**Decision summary** contributes only current known coverage to the book page;
**Vocabulary investment** and **Highest-impact unknown vocabulary** merge with
their book-page equivalents and appear once. **Identity and trust**,
**Projected token coverage**, **Vocabulary state and exclusions**,
**Structural and text signals**, and **Provenance and history** are retired from
the learner surface.

`GET /jobs/{id}` continues to own queued/running/cancelled/failed progress,
retry, and recovery. A completed scoped job's **View analysis result** action
opens the book page, directly or through the compatibility redirect; Journey
members reach their Journey context through that redirect.
`GET /jobs` remains the operational analysis-history surface.

This decision changes the learner-facing contract in ADR 0028 without changing
its immutable source, confirmed-scope, analysis-run, corpus, preparation, or
artifact identities. Deck preparation remains bound internally to the exact
completed analysis that supplied its corpus.

## Alternatives considered

- **Keep the two pages and only refactor shared components.** This would reduce
  duplicated implementation but preserve duplicate learner surfaces and the
  run-selection model that caused the navigation burden.
- **Collapse everything onto one merged page and delete the exact-result route
  entirely.** This is conceptually simplest, but it would break existing deep
  links, completed-job actions, and deck-preparation references. A thin redirect
  preserves compatibility at little learner-facing cost.
- **Keep multiple learner-facing analyses with provenance.** This retains exact
  run comparison and historical scope identity, but makes operational history a
  primary learner concern and keeps the complexity this decision is intended to
  remove.

## Consequences

- Learners have one book-centered place to understand current coverage,
  vocabulary investment, important unknown vocabulary, and deck preparation.
- Reanalysis has replacement semantics on learner surfaces; it cannot create
  multiple competing “current” results.
- Existing deep links remain valid through a compatibility redirect, while
  `/jobs` and job detail retain operational history and recovery.
- Immutable analysis and preparation provenance remains available to backend
  logic and audit without displacing the learner's book context.
- Learners lose the ability to compare two scope choices side by side, and
  prior analyses are demoted to audit-only records.
- The book page carries more responsibility and must preserve a clear hierarchy
  between lifecycle state, the premier coverage metric, supporting investment
  and vocabulary evidence, warnings, and the deck action.

## Related

- [Book analysis and deck workflow](../design/workflows/book-analysis-and-deck.md)
- [Information architecture: Analysis continuity](../design/information-architecture.md#analysis-continuity)
- [Analysis Insights](../features/analysis-insights.md)
- [Product summary and ADR register](../product.md)
- [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](0028-explicit-scoped-analysis-lifecycle.md)
