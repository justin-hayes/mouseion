# Information architecture

Status: **Canonical learner-facing design direction.** This document defines the
accepted experience architecture, not a persistence model or a claim that every
surface is already shipped. Conflicts with current feature documents, ADRs, and
routes are explicit under
[Contract changes requiring planner/ADR work](#contract-changes-requiring-planneradr-work).

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
- **Reviewed scope** — immutable learner-confirmed source-unit selection.
- **Analysis run** — asynchronous analysis attempt; queue and retry details are
  operational state.
- **Analysis result** — immutable completed corpus and provenance used by
  insights and deck preparation.
- **Prepared deck** — immutable APKG artifact from one exact analysis result.
- **Learning campaign** — current accepted domain object for one book/deck
  workflow and its vocabulary reservation/graduation semantics. It remains an
  internal or secondary concept until its contract is deliberately reconciled
  with Primary Goal.
- **Known vocabulary** — owner-scoped vocabulary explicitly imported or
  graduated through an accepted transition.
- **Catalog connection** — learner-owned OPDS endpoint and credentials.
- **Study language** — owner-scoped preference selected from capabilities
  advertised as ready by the NLP service.

A relationship graph, not a strict containment hierarchy, connects these
objects. A book can exist without a Journey or Primary Goal. A Journey entry
does not own a book or analysis. A Primary Goal does not make a projection
actual. An analysis result remains historically addressable after vocabulary or
Journey state changes.

## Primary navigation

The authenticated shell exposes three principal destinations:

- **My Books** — the canonical home and broad book collection;
- **Reading Journey** — the current Primary Goal, provisional sequence, route
  evidence, and Where next? transition;
- **Settings** — study languages and known vocabulary.

**Add books** is a persistent global workflow action, not a fourth destination.
It enters catalog setup and browsing and remains visually distinguishable from
navigation.

Primary Goal is never a separate top-level destination. Analysis jobs, deck
preparation, catalog connections, and campaign history are supporting surfaces.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

## Route and screen hierarchy

The hierarchy below identifies conceptual ownership. It deliberately does not
choose new route names or storage APIs before planner/ADR work.

```text
Authentication
    sign in or first-account onboarding

My Books
    book detail
        scope review
        analysis status
        exact analysis result
        deck preparation and download

Reading Journey
    embedded Primary Goal, when present
    provisional ordered books
    route comparison and reorder preview
    completion outcome
    Where next?

Add books
    acquisition hub
    catalog connection setup/maintenance
    catalog browse, search, and acquisition

Settings
    study languages
    known vocabulary and import

Secondary history
    operational analysis history
    reading, preparation, and vocabulary-transition history
```

### Current route compatibility

The current application uses `/library` for its owned-book collection and
`/campaigns` for the active-campaign queue/history screen. `/` redirects to
`/library`. Those routes are implementation facts, not permission to retain My
Library, Learning, queue, or Campaign as primary learner-facing concepts.

Whether `/library` is retained for My Books, whether `/campaigns` redirects or
is replaced, and what route owns Reading Journey are planner/implementation
questions. New templates must not invent a second competing navigation system
while that work is unresolved. ADR 0034 decides the queue side of this: the
derived Campaign queue is retired in favour of Reading Journey, `GET /campaigns`
redirects to the Journey surface, and book-centered Campaign actions (history,
graduation control) are reachable from the book detail / operational surfaces
without presenting a second learner-facing plan.

Existing nested analysis and artifact routes remain secondary surfaces:

```text
/books/{id}
/books/{id}/scope
/books/{id}/analyze
/books/{id}/analyses/{analysis-run-id}
/jobs/{id}
/deck-preparations/{id}/status
/deck-preparations/{id}/download
/connections
/catalog and /opds/*
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

Book detail remains the place for full lifecycle state, exact analysis history,
scope, provenance, and actions. My Books should be moderately dense and should
not place every book in a large card.

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
without blocking **Where next?** No next Goal is automatic. Implementing that
role transition against the current Campaign contract requires planner/ADR work.

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

The canonical completed-analysis destination remains the book-centered exact
result:

```text
/books/{book-id}/analyses/{analysis-run-id}
```

It identifies the book, confirmed scope, immutable analysis, quality state,
insights, and eligible deck action. The operational job page remains responsible
for queued/running progress, cancellation, retry, attempts, and failure
recovery. When work completes, its primary action is **View analysis result**.

An analysis result answers questions in this order:

1. identity and trust;
2. current scoped coverage and the most relevant conditional projection;
3. vocabulary investment;
4. structural context kept separate from lexical coverage;
5. provenance and history;
6. available next actions.

Deck preparation follows material warnings and the evidence summary. It does
not automatically add a book to Reading Journey, choose a Primary Goal, or mark
vocabulary known.

## Settings ownership

Settings owns study-language preferences and known vocabulary. Under the current
contract, removing a study language removes only the preference; it does not
delete books, analyses, decks, internal Campaigns, or known vocabulary.
Known-vocabulary import remains additive and does not imply a correction or
reversal path. Future Journey/Goal relationships must not be described as
deleted or preserved until their contract exists.

## Contract changes requiring planner/ADR work

The learner-facing architecture above is accepted. The following product/domain
questions remain deliberately unresolved and must be handled through the normal
feature-planning and ADR process before implementation. This document does not
choose tables, identifiers, APIs, migrations, or compatibility behavior.

Broader My Books membership is resolved by
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md): an
owner-scoped bibliographic Book and its My Books membership are distinct from
immutable acquired source evidence. Its staged implementation remains pending
and must preserve the shipped acquisition contract during compatibility rollout.

1. **Reading Journey identity and ordering.** Current contracts have a campaign
   queue, not provisional learner-selected membership, free ordering, or
   historical/current route comparison. Persistence, concurrency, and stale
   recalculation behavior require a product contract.
Resolved by [ADR 0034: One implicit Reading Journey with learner-canonical
   ordering and campaign-queue migration](../adr/0034-reading-journey-identity-ordering.md).
2. **Primary Goal identity.** ADR 0027 requires a prepared deck before a
   Campaign exists. Primary Goal must support commitment before analysis or
   deck preparation and possibly reading without Anki. Its relationship to an
   internal Campaign is unresolved.
   Resolved by [ADR 0036: Deck-independent Primary Goal and single justified
   vocabulary-graduation transition](../adr/0036-primary-goal-justified-graduation.md):
   one deck-independent Primary Goal per learner, meaningful before analysis or
   deck and readable without Anki; a Goal never reserves vocabulary itself, and
   Campaign remains the internal reservation/graduation mechanism.
3. **Completion and vocabulary graduation.** ADR 0027 atomically completes a
   Campaign only after both book-finished and deck-reviewed facts, then
   graduates assigned vocabulary. The accepted experience treats reading
   completion as a factual outcome even when vocabulary work remains. The
   transition and copy cannot be split or relabeled without revisiting that
   contract.
   Resolved by [ADR 0036](../adr/0036-primary-goal-justified-graduation.md):
   reading-finished is independent of deck-reviewed, graduation happens only
   through the single justified transition (snapshotted,
   provenance-linked identities + confirmed review), and the four-beat finish
   states current-versus-conditional honestly.
4. **A new Goal while vocabulary work remains.** The accepted Where next?
   experience permits reconsideration after the book is finished, while the
   current single-active Campaign may still reserve vocabulary. Whether another
   Goal can become current, and how reservation/projection semantics behave,
   requires an explicit decision.
   Resolved by [ADR 0036](../adr/0036-primary-goal-justified-graduation.md):
   a new Goal triggers an explicit graduate-or-abandon resolution of the
   residual reservation, preserves the one-active-campaign exclusivity, and
   keeps overlap deterministic; no silent state change.
5. **Queue replacement and history.** The provisional Journey must not coexist
   with a learner-facing commitment queue. Migration or compatibility for
   queued, active, complete, and abandoned Campaign records requires planning;
   historical evidence must remain understandable.
Resolved by [ADR 0034](../adr/0034-reading-journey-identity-ordering.md):
   queued records migrate into Journey membership in creation order, active
   records remain the current Campaign, and completed/abandoned records remain
   history.
6. **Cross-book projection and route comparison.** The exact optimization
   objective, eligible evidence, threshold assumptions, transition assumptions,
   handling of incomparable books, and invalidation rules need a reproducible
   product/analysis contract. No composite score should be invented.
   Resolved by [ADR 0037: Cross-book vocabulary projection and advisory Journey
   ordering](../adr/0037-cross-book-projection-advisory-ordering.md): the
   vocabulary-efficient alternative optimizes exactly one named lexical property
   (current known-token coverage) over the learner-selected comparable books and
   fixed-order constraints, keeps current and conditional projected states
   distinct, leaves incomparable books at the learner's position without a
   fabricated rank, recalculates on demand, and never overrides the canonical
   learner order or invents a composite score.
7. **Routes and terminology rollout.** My Books and Reading Journey need one
   coherent navigation model across redirects, deep links, breadcrumbs, and
   tests. Route names are implementation decisions; learner-facing terminology
   must not drift during staged rollout.

Until these contracts are accepted, existing feature documents and ADRs remain
authoritative for domain behavior. Canonical design language may describe the
target experience, but must not be used to conceal a semantic mismatch.

## Cross-linking rules

Every principal or nested screen makes clear:

1. which book, Goal, Journey, analysis, or setting is in context;
2. which facts are current and which are conditional;
3. the learner's available next decision and why it is available;
4. how to return to the parent context without reconstructing the route through
   global navigation.

Operational identifiers, attempt counts, provenance, and classifier versions
remain available where useful, but they must not displace book title, author,
learner intention, or the next meaningful choice.
