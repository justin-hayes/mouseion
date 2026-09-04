# Information architecture

Status: **Canonical learner-facing architecture.** This document includes the
target one-current-analysis contract proposed in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md), which remains
unshipped until its implementation issues land. ADRs continue to own
persistence and historical decision details.

Mouseion is organized around literature the learner cares about, one current
reading commitment, and the changed possibilities that follow from justified
learning. It foregrounds books, bibliographic identity, learner intention, and
the next decision rather than backend jobs, preparation mechanics, or a generic
dashboard.

## Learner goals

The product supports these top-level goals:

1. keep a broad personal collection of books Mouseion knows about;
2. choose and revise a provisional Reading Journey;
3. commit to finishing one Primary Goal at a time, when desired;
4. understand trustworthy current and conditional preparation evidence;
5. review how actual vocabulary changes affect books ahead;
6. maintain study languages and known vocabulary.

The recurring experience rhythm is:

```text
My Books
    -> shape or reconsider Reading Journey
    -> choose one Primary Goal
    -> prepare and read without conflating those facts
    -> finish the book
    -> apply only justified vocabulary transitions
    -> recalculate the books ahead from actual state
    -> Where next?
```

This rhythm does not imply a required pipeline for every book. Acquisition,
scope confirmation, analysis, deck preparation, Journey membership, Primary
Goal choice, reading completion, and vocabulary graduation remain separate
transitions.

## Principal learner-facing model

### My Books

**My Books** is the broad collection of books Mouseion knows about for the
learner. It can include:

- acquired and metadata-only books;
- assessed and unassessed books;
- currently desired, formerly desired, and low-interest books;
- books in or outside Reading Journey;
- the current Primary Goal;
- completed books;
- books with stale, questionable, incomplete, or unavailable evidence.

My Books is a searchable bibliographic catalogue, not a readiness ranking or a
list of obligations. Title, author, edition when relevant, and learner intent
precede analysis status. Processing state appears only to explain available
evidence or the next relevant action.

The catalogue is browsable by language pills, including an unknown-language
bucket, and supports paging plus global text search across the local collection.
For a fresh account, its empty state explains catalogue setup and enters
`/connections`; catalogue setup and sync maintenance do not become a
destination, and no acquisition action appears in the top navigation. Synced
catalogue metadata is browsed only here.

### Reading Journey

**Reading Journey** is a fluid, provisional ordering of learner-selected books
they currently imagine reading. It is not a queue, curriculum, project plan,
calendar, or promise to finish the sequence.

The Journey:

- may be empty;
- has explicit and reversible membership;
- can be reordered freely;
- has no final destination or completion state;
- has no dates, overdue states, or progress percentage;
- preserves one clear learner order;
- may compare that order with a vocabulary-efficient alternative using only the
  same learner-selected books —
  [ADR 0037](../adr/0037-cross-book-projection-advisory-ordering.md) fixes that
  comparison's reproducible objective, transitions, and invalidation rules;
- responds to changes with neutral recalculation, not warnings;
- keeps unassessed or incomparable books visible without inventing readiness.

The first provisional book is a natural candidate for a future Primary Goal,
not an automatic commitment or recommendation.

Journey identity, owner-scoped membership, learner-canonical ordering,
concurrency, stale-write behavior, and the migration of Campaign queue/history
into the Journey are decided in
[ADR 0034](../adr/0034-reading-journey-identity-ordering.md).

### Primary Goal

**Primary Goal** is the one book the learner currently intends to finish. It is
the only meaningful commitment in the principal architecture and is embedded
at the beginning of Reading Journey rather than exposed as a peer destination.
A learner may have no Primary Goal.

The Primary Goal brings together the book's bibliographic identity and the
ordinary facts needed to serve the current undertaking:

- reading state;
- preparation and vocabulary-work state;
- current evidence and explicitly conditional projections;
- the next available learner decision.

It does not erase the distinction between reading, deck review, vocabulary
knowledge, analysis provenance, or prepared artifacts. It also does not make
later Journey books committed.

## Supporting product objects

These objects remain important, but they do not define principal navigation:

- **Book** — owner-scoped learner-facing bibliographic identity and the center
  of My Books membership, reading, analysis, and Journey relationships. It may
  exist without acquired content; ADR 0035 defines its identity and lifecycle.
- **Source snapshot** — immutable acquired EPUB content and extracted units;
  provenance rather than a destination. It is linked after acquisition and is
  never a metadata-only placeholder.
- **Reviewed scope** — immutable learner-confirmed source-unit selection. Scope
  review presents reliable top-level EPUB 3 TOC entries as an all-on checklist,
  or readable persisted units in flat spine order when projection is
  unreliable; TOC entries expand to existing unit IDs.
- **Analysis run** — asynchronous analysis attempt; queue and retry details are
  operational state.
- **Current analysis** — the one completed analysis whose evidence is presented
  on the book page. Its immutable corpus and provenance remain backend facts.
- **Prepared deck** — immutable APKG artifact from the exact analysis that
  supplied its corpus, even though preparation begins on the book page.
- **Learning campaign** — current accepted domain object for one book/deck
  workflow and its vocabulary reservation/graduation semantics. It remains an
  internal or secondary history/operations concept, not a learner-facing plan.
- **Known vocabulary** — owner-scoped vocabulary explicitly imported or
  graduated through an accepted transition.
- **Catalog connection** — learner-owned OPDS endpoint and credentials.
- **Catalogue sync** — periodic metadata reconciliation for one learner-owned
  connection. It is upsert-only and never destructive; content is trusted
  immutable for sync, so it never invalidates scope or analysis. The target
  contract is proposed by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md).
- **Study-language preference** — owner-scoped catalogue-sync selection from
  capabilities advertised as ready by the NLP service.
- **Library study language** — distinct normalized language tags of the owner's
  active chosen-language Books; it defines known-vocabulary language context.
- **Language lens** — a derived, evidence-only per-language aggregate over
  current analyses and known vocabulary. It owns no Book, scope, analysis, or
  action; [ADR 0042](../adr/0042-derived-language-corpus-view.md) proposes its
  target contract.

A relationship graph, not a strict containment hierarchy, connects these
objects. A book can exist without a Journey or Primary Goal. A Journey entry
does not own a book or analysis. A Primary Goal does not make a projection
actual. Earlier analysis runs remain addressable as operational audit records
after vocabulary, Journey, source, or scope changes; they are not parallel
learner result surfaces.

## Primary navigation

The authenticated shell exposes three principal destinations:

- **My Books** — the canonical home, broad book collection, and sole browse
  surface for synced catalogue metadata;
- **Reading Journey** — the current Primary Goal, provisional sequence, route
  evidence, and Where next? transition;
- **Settings** — study languages and known vocabulary.

The top navigation has no acquisition action. Catalogue setup and sync
maintenance are supporting `/connections` routes reached from My Books empty
states and actions; My Books is the sole browse surface and Book detail owns
per-book EPUB acquisition. The upstream catalog browser is retired.

Primary Goal is never a separate top-level destination. Analysis jobs, deck
preparation, catalog connections, and campaign history are supporting surfaces.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

There is no separate **Catalogues / Browse** sub-navigation. `/connections` is
for connection and sync maintenance; `/library` is the canonical local browse
surface.

## Route and screen hierarchy

The hierarchy below records shipped conceptual and route ownership. It does not
replace the ADRs' detailed storage and compatibility decisions.

```text
Authentication
    sign in or first-account onboarding

My Books
    book detail
        scope review
        analysis status
        current analysis insights and deck-preparation action
        exact-analysis compatibility redirect
        deck preparation and download

Reading Journey
    embedded Primary Goal, when present
    provisional ordered books
    route comparison and reorder preview
    completion outcome
    Where next?

Catalogue maintenance
    connection setup and sync status
Book detail
    per-book EPUB acquisition

Settings
    study languages
    known vocabulary and import

Secondary history
    operational analysis history
    reading, preparation, and vocabulary-transition history
```

### Current route compatibility

The shipped application uses `/library` for My Books and `/journey` for Reading
Journey. `/` redirects to `/library`, while `GET /campaigns` is a compatibility
redirect to `/journey`. The Journey page retains a secondary **Campaign history
& operations** section for prepared-deck actions and reading, preparation, and
vocabulary-transition history. It does not present a duplicate queue or plan.

The authenticated shell therefore exposes exactly My Books, Reading Journey, and
Settings in the top navigation. `/connections` (catalogue maintenance and sync)
and `/jobs` are supporting surfaces reached from My Books and direct routes, not
navigation destinations. Historical My Library, Learning,
queue, and learner-facing Campaign labels are not active navigation concepts;
compatibility aliases and operational terminology remain only where required by
existing routes, records, or infrastructure.

Existing nested analysis and artifact routes remain supporting routes. The
run-specific analysis route is a compatibility redirect rather than a separate
surface:

```text
/books/{id}
/books/{id}/scope
/books/{id}/analyze
/books/{id}/analyses/{analysis-run-id}
/jobs/{id}
/deck-preparations/{id}/status
/deck-preparations/{id}/download
/connections
/opds/acquire (per-book content acquisition)
/settings and known-vocabulary support routes
```

Mutation, fragment, and JSON status endpoints support a parent screen; they are
not learner-facing destinations.

## My Books information hierarchy

My Books answers questions in this order:

1. What literature do I care about or want to find again?
2. Which book is my Primary Goal, and which are in my Journey?
3. Which books have trustworthy current evidence?
4. Which books are unassessed, stale, or cannot currently be assessed?
5. What can I do with this book next?

Rows lead with title, author, and edition/year/language where useful. Learner
intent and Journey/Goal relationship precede concise evidence state. Search,
filtering, and sorting support finding books but do not turn readiness into the
default ranking.

Book detail remains the place for full lifecycle state, the one current
analysis, and its actions. Exact analysis history and provenance are operational
facts available through `/jobs`, not sections on the learner-facing book page.
My Books should be moderately dense and should not place every book in a large
card.

## Reading Journey information hierarchy

Reading Journey answers questions in this order:

1. What am I committed to finishing now, if anything?
2. What do I currently imagine reading after it?
3. Which order is mine and how can I change it?
4. What current and conditional preparation evidence is trustworthy?
5. How would a vocabulary-efficient alternative change one stated lexical
   property?
6. What happens if I keep my preference?

The Primary Goal is visually distinct but remains book-led rather than a large
metric card. Everything after it is explicitly provisional and directly
reorderable with keyboard-operable controls. Drag may enhance but never replace
**Move earlier** and **Move later**.

The learner's order is always the active order. A vocabulary-efficient
alternative is optional comparison evidence. Its method, planning threshold,
scopes, evidence recency, and vocabulary-transition assumptions must be
available. Aggregate totals are supporting detail; lead with book order and the
plain-language consequence.

Unassessed or incomparable books stay in the Journey at the learner's chosen
position. Mouseion explains the evidence gap and excludes them from totals
rather than moving or demoting them silently.

## Primary Goal completion and Where next?

Reading completion and vocabulary knowledge are independent facts. When reading
is finished, the book no longer occupies the current Primary Goal role; it
remains in My Books and history, and unfinished vocabulary work remains visible
without blocking **Where next?** No next Goal is automatic. ADR 0036 defines the
shipped independent reading and justified vocabulary transitions.

Completion proceeds in four beats:

1. state the factual reading outcome;
2. state the justified vocabulary transition or its explicit absence;
3. replace old forecasts with recalculation from actual state and show what
   changed or remained conditional;
4. return attention to Reading Journey with **Where next?**

Two required branches are:

- **Reading finished and the justified vocabulary transition is complete.** Add
  only eligible vocabulary through the accepted transition, recalculate later
  books from actual known vocabulary, and show precise before/after evidence.
- **Reading finished while vocabulary work remains.** Acknowledge the reading
  achievement, state that no vocabulary has yet been added to known, keep
  current values unchanged, and preserve any future effect as conditional.

The first remaining Journey book may be presented as **first in your current
order**, never **optimal next text**. **Choose as Primary Goal**, reorder, remove,
add from My Books, and choose another book are peer choices. No next Goal is
automatic. An empty Journey returns calmly to My Books; it is not a failed or
completed plan.

## Analysis continuity

The book page at `/books/{id}` is the canonical home for the book's one current
analysis. The run-specific route
`/books/{book-id}/analyses/{analysis-run-id}` remains only as a compatibility
redirect to that book page, preserving deep links and exact-analysis references
without rendering a second insight surface.

The book page answers completed-analysis questions in this order:

1. What is my **Current known coverage** of the analyzed units?
2. What additional vocabulary would reach the documented coverage targets?
3. Which unknown vocabulary has the highest contribution?
4. Do any concrete analysis-quality gaps require a compact warning?
5. Do I want to prepare a deck from this analysis?

Current known coverage is the headline and premier metric, followed by
**Vocabulary investment** / **Additional vocabulary** and **Highest-impact
unknown vocabulary**. A one-line qualifier such as “of the analyzed units”
makes the coverage scope unambiguous without creating an **Analyzed scope**
section. The page renders one compact quality note only when the analyzer data
contains a concrete warning; clean analysis renders no quality region.

The learner surface does not show analyzed-scope details, text profile,
projected token coverage, the broader coverage-stat list, analysis history, or
run identity/provenance sections. Deck preparation follows the retained
evidence on the book page. It does not automatically add a book to Reading
Journey, choose a Primary Goal, or mark vocabulary known.

The operational job page remains responsible for queued/running progress,
cancellation, retry, attempts, and failure recovery. When work completes, its
primary action is **View analysis result**, which opens the book page directly
or through the compatibility redirect. `GET /jobs` remains the operational
history surface for current and prior runs.

## Settings ownership

Settings owns study-language preferences and known vocabulary. Under the current
contract, removing a study language removes only the preference; it does not
delete books, analyses, decks, internal Campaigns, or known vocabulary.
Known-vocabulary import remains additive and does not imply a correction or
reversal path. Journey and Goal relationships are independent of Settings and
known-vocabulary removal; their shipped consequences are defined by ADR 0034
and ADR 0036.

<a id="contract-changes-requiring-planneradr-work"></a>

## Architecture decisions and target reconciliation

The following decisions record shipped architecture and the proposed
one-current-analysis target. Accepted ADRs remain authoritative for persistence,
historical records, and compatibility details until ADR 0040 is accepted and
implemented.

Broader My Books membership is resolved by
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md): an
owner-scoped bibliographic Book and its My Books membership are distinct from
immutable acquired source evidence. The shipped acquisition path creates or
restores membership only after the validated source snapshot is persisted.

1. **Reading Journey identity and ordering** are resolved by [ADR 0034: One
   implicit Reading Journey with learner-canonical ordering and campaign-queue
   migration](../adr/0034-reading-journey-identity-ordering.md). The shipped
   Journey is the single learner-facing order, and `/campaigns` redirects to it.
2. **Primary Goal identity** is resolved by [ADR 0036: Deck-independent Primary
   Goal and single justified vocabulary-graduation transition](../adr/0036-primary-goal-justified-graduation.md):
   one deck-independent Primary Goal per learner is meaningful before analysis or
   deck preparation, and Campaign remains secondary reservation state.
3. **Completion and vocabulary graduation** use the independent reading-finished
   fact and the single justified transition defined by [ADR 0036](../adr/0036-primary-goal-justified-graduation.md).
   The finish outcome states current versus conditional evidence honestly.
4. **Residual vocabulary work and a new Goal** follow ADR 0036: a new Goal is
   explicit, residual reservations require an explicit graduate-or-abandon
   resolution before new reserved work, and overlap remains deterministic.
5. **Campaign queue replacement and history** follow ADR 0034: the Journey is
   the only learner-facing plan, while active, completed, and abandoned Campaign
   records remain available as secondary history/operations and provenance.
6. **Cross-book projection and route comparison** follow [ADR 0037: Cross-book
   vocabulary projection and advisory Journey ordering](../adr/0037-cross-book-projection-advisory-ordering.md):
   the alternative is advisory evidence only and never overrides learner order
   or invents a composite score.
7. **Routes and terminology** are reconciled in the shipped shell and supporting
   surfaces: My Books, Reading Journey, and Settings are the active navigation
   destinations, with no acquisition action in the top navigation; `/known-vocab`
   and `/campaigns` remain compatibility
   routes with their documented redirects.
8. **One current analysis per book** is the target contract proposed by
   [ADR 0040](../adr/0040-one-current-analysis-per-book.md): book detail becomes
   the sole learner-facing insight surface, prior runs remain operational audit
   records, and run-specific result URLs redirect to the book.
9. **Catalogue sync** is the target contract proposed by
   [ADR 0041](../adr/0041-catalog-sync-metadata-first.md): each learner-owned
   connection periodically reconciles metadata for ready study languages except
   English, without downloading content, deleting local state, or invalidating
   scope or analysis.
10. **Derived language corpus lens** is the target contract proposed by
    [ADR 0042](../adr/0042-derived-language-corpus-view.md): an evidence-only
    per-language panel starts within My Books and derives aggregates from current
    analyses and known vocabulary. It may become a destination only after future
    explicit reconciliation at this checkpoint.

## Cross-linking rules

Every principal or nested screen makes clear:

1. which book, Goal, Journey, analysis, or setting is in context;
2. which facts are current and which are conditional;
3. the learner's available next decision and why it is available;
4. how to return to the parent context without reconstructing the route through
   global navigation.

Operational identifiers, attempt counts, and historical provenance remain
available where useful, but they must not displace book title, author, learner
intention, or the next meaningful choice. Classifier and recommendation data
are compatibility history, not current scope-review concepts.
