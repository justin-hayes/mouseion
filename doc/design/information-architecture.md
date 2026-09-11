# Information architecture

Status: **Canonical shipped learner-facing architecture.** This document follows
the one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent analysis trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md), with the
standalone learner action retired by [ADR 0054](../adr/0054-retire-standalone-analysis-action.md).
Language is the
app's organizing mode: [ADR 0050](../adr/0050-active-study-language.md) scopes
every language-dependent surface to one active study language, and
[ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md) makes Reading
Journey and Primary Goal one per language, and catalog maintenance is a fourth
principal destination per
[ADR 0058](../adr/0058-catalog-maintenance-principal-destination.md). ADRs
continue to own persistence and historical decision details.

Mouseion is organized around literature the learner cares about, one current
reading commitment, and the changed possibilities that follow from justified
learning. It foregrounds books, bibliographic identity, learner intention, and
the next decision rather than backend jobs, preparation mechanics, or a generic
dashboard.

## Learner goals

The product supports these top-level goals:

1. keep a broad personal collection of books Mouseion knows about;
2. work in one active study language at a time;
3. choose and revise a provisional Reading Journey in that language;
4. commit to finishing one Primary Goal per language, when desired;
5. understand trustworthy current and conditional preparation evidence;
6. review how actual vocabulary changes affect books ahead;
7. understand derived study languages and maintain known vocabulary;
8. maintain the learner-owned catalog connections that feed My Books.

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

This rhythm does not imply a required pipeline for every Book. Catalog sync,
analysis, deck preparation, Primary Goal choice, reading completion, and
vocabulary graduation remain separate transitions. The learner-initiated **Add
to Reading Journey** action is the analysis exception: it adds membership and
intentionally acquires the current EPUB and ensures whole-book analysis as one
backlog action. My Books metadata refresh remains separate and never starts
analysis.

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

My Books is a searchable bibliographic catalog, not a readiness ranking or a
list of obligations. Title, author, edition when relevant, and learner intent
precede analysis status. Processing state appears only to explain available
evidence or the next relevant action.

The catalog is browsed within the active study language: browse, paging, and
search are scoped to it, and no "All languages" default exists. Books without a
chosen language belong to no language partition and are surfaced only through
an out-of-band **needs language** strip (fix the language in the catalog, then
re-sync; display-only, no per-book actions). For a fresh account, its empty
state explains catalogue setup and enters `/catalogs`; catalogue setup and sync
maintenance are owned by the Catalogs destination. Synced catalogue metadata is
browsed only here.

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

There is one Journey per study language, created lazily and shown for the
active language; a Book with a chosen language joins its language's Journey,
and an unknown-language Book joins none. Journey identity, owner-scoped
membership, learner-canonical ordering, concurrency, stale-write behavior, and
the migration of Campaign queue/history into the Journey are decided in
[ADR 0034](../adr/0034-reading-journey-identity-ordering.md) and
[ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md).

### Primary Goal

**Primary Goal** is, per study language, the one book in that language's
Reading Journey the learner currently intends to finish. It is the only
meaningful commitment in the principal architecture and is embedded at the
beginning of its Journey rather than exposed as a peer destination. A learner
may have no Primary Goal; how many Goals are active across languages is the
learner's own discipline, not an enforced invariant.

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
  on the Journey entry. Its immutable corpus and provenance remain backend facts.
- **Prepared deck** — immutable APKG artifact from the exact analysis that
  supplied its corpus, even though preparation begins on the Journey entry.
- **Book vocabulary study** — owner-scoped reservation and graduation state
  attached to a prepared deck for one Book. It is reached from the Journey entry,
  not represented as a separate plan or campaign object.
- **Known vocabulary** — owner-scoped vocabulary explicitly imported or
  graduated through an accepted transition.
- **Catalog connection** — learner-owned OPDS endpoint and credentials.
- **Catalog sync** — periodic metadata reconciliation for one learner-owned
  connection. It is upsert-only and never destructive; content is trusted
  immutable for sync, so it never invalidates scope or analysis. The metadata
  contract is defined by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md)
  and its language scope is reconciled by ADR 0043.
- **Study language** — a distinct normalized language tag of the owner's active
  chosen-language Books; it defines known-vocabulary language context and is
  derived rather than selected in Settings.
- **Active study language** — the one study language the learner is currently
  working in; a stored selection pointing into the derived set that scopes every
  language-dependent surface ([ADR 0050](../adr/0050-active-study-language.md)).
- **Language lens** — historical terminology for the retired panel proposed by
  [ADR 0042](../adr/0042-derived-language-corpus-view.md). It is not a current
  object or learner-facing surface; [ADR 0057](../adr/0057-retire-language-view-panel.md)
  records the retirement.

A relationship graph, not a strict containment hierarchy, connects these
objects. A book can exist without a Journey or Primary Goal. A Journey entry
does not own a book or analysis. A Primary Goal does not make a projection
actual. Earlier analysis runs remain addressable as operational audit records
after vocabulary, Journey, source, or scope changes; they are not parallel
learner result surfaces.

## Primary navigation

The authenticated shell exposes four principal destinations:

- **My Books** — the canonical home, broad book collection, and sole browse
  surface for synced catalog metadata;
- **Reading Journey** — the current Primary Goal, provisional sequence, route
  evidence, and Where next? transition;
- **Vocabulary** — known vocabulary and its import workflow.
- **Catalogs** — learner-owned catalogue connections and metadata sync.

Catalogs is a configuration and sync destination, not a book-browse surface.
My Books remains the sole browse surface and its rows own the Reading Journey
acquisition-and-analysis intent. The upstream catalog browser is retired.

Primary Goal is never a separate top-level destination. Analysis jobs, deck
preparation, and per-Book vocabulary-study history are supporting surfaces.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shell also carries a native **active study language** control alongside the
four destinations. Changing it navigates to the same screen in the new language
on language-scoped screens and updates the stored mode elsewhere; it lists the
derived study languages plus any known-vocabulary-only language marked "no
books", marks a newly arrived study language "new", and never auto-switches on
navigation or sync.

There is no separate **Catalogues / Browse** sub-navigation. `/catalogs` is for
connection and sync maintenance; `/library` is the canonical local browse
surface.

## Route and screen hierarchy

The hierarchy below records shipped conceptual and route ownership. It does not
replace the ADRs' detailed storage and compatibility decisions.

```text
Authentication
    sign in or first-account onboarding

My Books
    needs-language strip (books awaiting a language)
    metadata refresh on eligible rows
    add/remove Reading Journey membership

Reading Journey (active study language)
    embedded Primary Goal, when present
    provisional ordered books
    Journey entry for current completed analysis
        current analysis insights and deck-preparation action
        exact-analysis compatibility redirect
    route comparison and reorder preview
    completion outcome
    Where next?

Catalogs
    connection setup and sync status
Journey entry
    current completed analysis view for a Journey member

Vocabulary
    known vocabulary and import

Secondary history
    operational analysis history
    reading, preparation, and vocabulary-transition history
```

### Current route compatibility

The shipped application uses `/library` for My Books and `/journey` for Reading
Journey. `/` redirects to `/library`. A Journey entry at `/journey/{bookID}`
owns the Book's current vocabulary-study state and a secondary per-Book study
history. The application does not present a duplicate queue, campaign, or plan.

The authenticated shell therefore exposes exactly My Books, Reading Journey,
Vocabulary, and Catalogs in the top navigation. `/jobs` remains a supporting
surface reached from direct routes, not a navigation destination. Historical My Library, Learning,
queue, and learner-facing Campaign labels are not active navigation concepts;
compatibility aliases and operational terminology remain only where required by
existing routes, records, or infrastructure.

Existing nested analysis and artifact routes remain supporting routes. The
run-specific analysis route is a compatibility redirect rather than a separate
surface:

```text
/journey/{bookID}
/books/{id}/analyses/{analysis-run-id} (compatibility redirect)
/jobs/{id}
/deck-preparations/{id}/status
/deck-preparations/{id}/download
/catalogs
/connections (compatibility redirect to `/catalogs`)
/settings (compatibility redirect to `/library`)
/vocabulary and known-vocabulary import support routes
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

Rows lead with title, author, and edition/year where useful; the language tag is
carried by the active-language heading, not repeated per row. Learner intent and
Journey/Goal relationship precede concise evidence state. Search is scoped to
the active language; filtering and sorting support finding books but do not turn
readiness into the default ranking.

Journey entries remain the place for full lifecycle state and the one current
analysis for members. Books without a reachable Journey entry remain in My
Books with their available row actions. Exact analysis history and provenance
are operational facts available through `/jobs`, not sections on the learner-facing
Journey entry. My Books should be moderately dense and should not place every
book in a large card.

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

The Journey entry at `/journey/{bookID}` is the canonical presentation for a
member's one current analysis. The run-specific route
`/books/{book-id}/analyses/{analysis-run-id}` remains only as a compatibility
redirect to the Journey entry for reachable members and returns 404 otherwise,
preserving valid deep links without rendering a second insight surface.

The Journey entry answers completed-analysis questions in this order:

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
evidence on the Journey entry. It does not automatically add a book to Reading
Journey, choose a Primary Goal, or mark vocabulary known.

The operational job page remains responsible for queued/running progress,
cancellation, retry, attempts, and failure recovery. When work completes, its
primary action is **View analysis result**, which opens the Journey entry directly
or through the compatibility redirect. `GET /jobs` remains the operational
history surface for current and prior runs.

## Active study language, study languages, and Vocabulary ownership

Study languages are derived from the distinct normalized language tags of the
learner's active chosen-language Books. For catalog-synced Books, the
catalog entry is the source of truth and connection re-sync is the only way
the language changes; there is no separate Settings selection to maintain.

The **active study language** is a stored selection pointing into that derived
set — context, not configuration: it chooses which study language the
language-scoped surfaces (My Books browse and search, Reading Journey,
Vocabulary) present, and never defines which languages are studied. It defaults
deterministically (the sole study language, else the language of the most
recently activated chosen-language Book) and resets lazily when the selection
leaves the set. A shell-level switcher carries it on every authenticated screen;
changing it navigates to the same screen in the new language on language-scoped
screens. Journey entries are not mode-scoped: they render a Book's own language and
never auto-switches the mode. A newly arrived study language appears passively
in the switcher (marked "new") without changing the mode.

Vocabulary owns known vocabulary and its additive import workflow, scoped to the
active language; import is always eligible there. Import eligibility remains
limited to the derived study-language set, and known-vocabulary-only languages
stay selectable in the switcher as read-only "no books" entries. Catalog
metadata changes do not delete known-vocabulary rows, books, analyses, decks, or
vocabulary-study history. Journey and Goal relationships remain independent of
vocabulary import; their shipped consequences are defined by ADR 0034, ADR 0036,
ADR 0050, ADR 0051, and ADR 0056.

<a id="contract-changes-requiring-planneradr-work"></a>

## Architecture decisions and target reconciliation

The following decisions record shipped architecture. Accepted ADRs remain
authoritative for persistence, historical records, and compatibility details.

Broader My Books membership is resolved by
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md): an
owner-scoped bibliographic Book and its My Books membership are distinct from
immutable acquired source evidence. Catalog sync creates metadata-only
membership; Reading Journey intent acquires validated source evidence when
needed.

1. **Reading Journey identity and ordering** are resolved by [ADR 0034: One
   implicit Reading Journey with learner-canonical ordering and campaign-queue
   migration](../adr/0034-reading-journey-identity-ordering.md). The shipped
   Journey is the single learner-facing order.
2. **Primary Goal identity and eligibility** use [ADR 0036: Deck-independent
   Primary Goal and single justified vocabulary-graduation transition](../adr/0036-primary-goal-justified-graduation.md)
   for Goal and graduation semantics, tightened by [ADR 0049: Reading intent
   triggers analysis](../adr/0049-reading-intent-triggers-analysis.md): one
   Primary Goal is promoted only from a Journey member with successfully
   completed current analysis, and it clears when that member leaves the Journey.
3. **Completion and vocabulary graduation** use the independent reading-finished
   fact and the single justified transition defined by [ADR 0036](../adr/0036-primary-goal-justified-graduation.md).
   The finish outcome states current versus conditional evidence honestly.
4. **Residual vocabulary work and a new Goal** follow ADR 0036: a new Goal is
   explicit, residual reservations require an explicit graduate-or-abandon
   resolution before new reserved work, and overlap remains deterministic.
5. **Campaign queue retirement and vocabulary study** follow [ADR 0056](../adr/0056-retire-campaign-learner-surface.md)
   and [ADR 0053](../adr/0053-book-anchored-vocabulary-consolidation.md): the
   Journey is the only learner-facing plan, and Book vocabulary-study history is
   shown on the Journey entry.
6. **Cross-book projection and route comparison** follow [ADR 0037: Cross-book
   vocabulary projection and advisory Journey ordering](../adr/0037-cross-book-projection-advisory-ordering.md):
   the alternative is advisory evidence only and never overrides learner order
   or invents a composite score.
7. **Routes and terminology** are reconciled in the shipped shell and supporting
    surfaces: My Books, Reading Journey, Vocabulary, and Catalogs are the active
    navigation destinations; `/known-vocab`
   remains a compatibility route with its documented redirect. Campaign routes
   are not learner-facing webapp routes.
8. **One current analysis per Book** follows [ADR 0040](../adr/0040-one-current-analysis-per-book.md):
   Journey entries are the current completed-analysis surface for members, prior
   runs remain operational audit records, and run-specific result URLs redirect
   to the applicable current context.
9. **Catalog sync** follows the accepted contract in
   [ADR 0041](../adr/0041-catalog-sync-metadata-first.md), with its language
   scope reconciled by [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
   each learner-owned connection periodically reconciles metadata for every
   non-English language whose NLP pipeline is ready, without downloading
   content, deleting local state, or invalidating scope or analysis.
10. **Language view retirement** follows [ADR 0057](../adr/0057-retire-language-view-panel.md):
    the panel proposed by ADR 0042 has no current route or screen contract.
    Per-Book evidence remains on My Books rows and current analysis evidence
    remains on Journey entries; no replacement aggregate is implied.
11. **Derived study languages and Vocabulary** are resolved by
    [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
    chosen-language Books define the language set, Vocabulary owns
    known-vocabulary import, and Settings is removed from primary navigation.
    Any future change to the derived-language source or Vocabulary's destination
    must return to this checkpoint.
12. **Active study language mode** is resolved by
    [ADR 0050](../adr/0050-active-study-language.md): one stored selection
    pointing into the derived set scopes My Books, Reading Journey, and
    Vocabulary; `?language=` params, the "All languages" pill, and per-row
    language tags are removed; a shell-level native switcher carries the mode;
    new languages arrive passively; legacy no-language Books surface only
    through a **needs language** strip.
13. **Per-language Journey and Goal** are resolved by
    [ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md): Reading
    Journey and Primary Goal identity are (owner, study language) with
    per-language revisions; the ADR 0037 route comparison is language-correct by
    construction; the legacy single Journey is split by a backfill migration
    under ADR 0038.

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
