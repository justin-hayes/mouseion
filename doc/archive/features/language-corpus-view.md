# Language Corpus View

Status: Historical / retired · Date: 2026-09-02 · Superseded by ADR 0057

> **Historical archive.** This document is retained for implementation history
> and is not part of the current product contract.

This document records the proposed Language view panel and is retained as
historical context. The panel and its read model were retired by
[ADR 0057](../../adr/0057-retire-language-view-panel.md); the requirements below
are not current product behavior.

## Motivation

Learning a lemma can change the current coverage of every analyzed book in the
same language. Per-book analysis shows each effect in isolation, so the learner
cannot see the cumulative shape of evidence across their collection. A compact
language-level lens can expose that relationship without changing what a Book,
scope, analysis, or deck means.

## Goal

Show current, private, same-language evidence across the learner's analyzed
Books as a derived panel in My Books, with a clear path back to each Book's
canonical evidence and actions.

## Scope

This feature defines the read model and learner-facing panel proposed by
[ADR 0042](../../adr/0042-derived-language-corpus-view.md). **Language corpus view**
is the internal feature name; learner-facing labels use **Language view**,
**Analyzed books**, or **Coverage across &lt;language&gt;**, never **Corpus**.

## Requirements

### Inputs and derivation

For one owner and one language, derive the panel at render time from:

- each Book's one current analysis under ADR 0040;
- its normalized corpus artifact, restricted to lemma/statistical data under
  ADR 0009; and
- the owner's current known vocabulary for that language.

Reuse ADR 0037's current-vocabulary rules and exact occurrence arithmetic:
graduated/imported known vocabulary contributes to coverage; generated or
reserved vocabulary is not silently known; an empty-UPOS lemma wildcard matches
all UPOS identities for that lemma. The read model is never shared across owners
or languages and is not persisted as a learner mastery snapshot.

### Initial evidence

The starting panel shows:

1. **Analyzed books** — the count of Books with a usable current analysis in the
   active study language.
2. **Aggregate known coverage** — known analyzable occurrences divided by total
   analyzable occurrences across the included analyses, with token weighting and
   scope/current-state qualification stated.
3. **Highest-impact unknown vocabulary** — unknown lemma identities ordered by
   their total occurrence contribution across the included analyses, with
   deterministic tie-breaking.
4. **Per-book spread** — each included Book's title, author where available,
   current known coverage, and relevant evidence state.

Books with missing, stale, incomplete, or non-reproducible evidence are excluded
from numeric aggregates and explained rather than assigned zero coverage. Their
status may remain visible so the lens does not hide collection membership.

### Placement and zoom

- The initial surface is a per-language panel inside My Books, associated with
  the active study language ([ADR 0050](../../adr/0050-active-study-language.md)).
- The panel remains subordinate to the searchable bibliographic collection; it
  is not a dashboard hero or a new navigation destination.
- Each analyzed Journey-member row links to `/journey/{bookID}`; Books without a
  reachable Journey entry remain evidence-only rows without a dead link.
- Promotion to a separate destination requires a future explicit information-
  architecture reconciliation.

### Hard boundaries

- **Evidence only:** the panel contains no scope, analyze, lazy-acquire,
  prepare-deck, or vocabulary-mutation action.
- **Separate book evidence:** it never combines reviewed scopes, creates an
  aggregate analysis or deck, or claims a shared analysis result.
- **Derived, not a new object:** it creates no persisted learner Corpus or
  collection lifecycle.
- **Private:** it performs no cross-learner pooling of source text, sentences,
  normalized results, vocabulary, or aggregates.

## States

| State | Required presentation | Primary exit |
|---|---|---|
| No analyzed books in language | Explain that no current comparable evidence is available without turning the panel into an analysis prompt. | Browse the language's Books |
| Some analyzed, some unavailable | Show aggregates only for included Books and list exclusions/reasons separately. | Open a Journey entry |
| Current evidence available | Show the four starting quantities with their owner/language/current-state basis. | Open a Journey entry |
| Evidence stale or incomplete | Remove the affected Book from aggregate arithmetic and identify why. | Review the affected Journey card |
| Known vocabulary changed | Recompute from current state; do not display a persisted prior value as current. | Continue browsing |
| Long vocabulary or book list | Page or truncate with an accessible explicit expansion while retaining deterministic order. | Open a Journey entry or continue browsing |

## Non-goals

- Actions in the lens; all lifecycle and preparation actions remain on the
  Journey entry or My Books row as appropriate.
- Aggregate decks, cross-book scopes, merged analyses, or a persisted corpus
  object.
- Cross-learner data or shared learner evidence.
- Recommendations, readiness rankings, or literary judgments.
- Learner-facing use of the word **Corpus**.
- Destination promotion without the documented architecture reconciliation.
- Finalizing the exact display quantities or graduation criteria for a future
  destination; the initial four quantities require validation.

## Acceptance criteria

- The read model is owner- and language-isolated and uses at most one current
  analysis per Book.
- Fixture calculations reproduce analyzed count, token-weighted aggregate known
  coverage, highest-contribution unknowns, and per-book values deterministically.
- Missing or stale evidence is excluded and explained, never treated as zero.
- The My Books language panel uses learner-facing wording other than **Corpus**
  and contains no consequential action.
- Every included analyzed Journey member can be opened at `/journey/{bookID}`;
  other Books remain evidence-only.
- No persisted corpus object, combined scope, aggregate deck, or cross-learner
  query is introduced.
- The panel is usable with keyboard and server-rendered navigation before any
  progressive enhancement.
