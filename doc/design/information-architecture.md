# Information architecture

Status: **Canonical route and navigation contract.** Some product-model sections
retain legacy terms to describe the accepted data model; active Reading screens
use current-Book and To Read language. ADR 0074 assigns deck work to the focused
preparation task. ADR 0072 continues to own snapshot and forecast semantics. This document follows
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
3. move Books into or out of To Read and revise their order;
4. choose a current Book to finish, when desired;
5. understand current, after-current-reading, and on-arrival coverage;
6. accept a finished Book's frozen vocabulary into the model used by later books;
7. understand derived study languages and import known vocabulary;
8. maintain the learner-owned catalog connections that feed My Books.

The recurring experience rhythm is:

```text
My Books
    -> move Books into To Read and revise their order
    -> choose a current Book
    -> prepare and read without conflating those facts
    -> finish the book
    -> apply only justified vocabulary transitions
    -> record the exact vocabulary counts
    -> choose what to read next from the candidate chooser
```

This rhythm does not imply a required pipeline for every Book. Catalog sync,
analysis, To Read disposition, current-reading selection, reading completion,
and prepared-deck artifact work remain distinct transitions. Moving a Book to To
Read is the analysis exception: the learner's intent intentionally acquires the
current EPUB and ensures whole-book analysis as one action. My Books metadata
refresh remains separate and never starts analysis. Starting current reading
freezes the vocabulary snapshot; finishing accepts it into modeled Known
vocabulary.

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
- shows current, after-Goal, and on-arrival coverage for that order —
  [ADR 0072](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md) fixes
  the snapshot, forecast, lower-bound, and invalidation rules;
- responds to changes with neutral recalculation, not warnings;
  - keeps unassessed or otherwise untrustworthy books visible without inventing readiness.

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

It keeps reading, Goal-owned vocabulary transitions, analysis provenance, and
prepared artifacts distinct. Historical deck and graduation provenance remains
available without creating a separate study workflow, and later Journey books
are not committed.

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
- **Current analysis** — the one completed analysis whose evidence supports the
  Book anchor in Reading Journey. Its immutable corpus and provenance remain
  backend facts.
- **Prepared deck** — immutable APKG artifact from the exact analysis that
  supplied its corpus, prepared through the focused deck task.
- **Goal vocabulary snapshot** — the immutable recurring-vocabulary identity set
  owned by an active Primary Goal, with its selection and analysis provenance.
- **Reserved vocabulary** — the active Goal snapshot projected into selection
  state for that study language; it is not a separate study workflow.
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
objects. A book can exist without a Journey or Primary Goal. Reading Journey
owns the Book anchor but does not own the Book or analysis. A Primary Goal does
not make a projection actual. Earlier analysis runs remain addressable as
operational audit records after vocabulary, Journey, source, or scope changes;
they are not parallel learner result surfaces.

## Primary navigation

The authenticated shell exposes four principal destinations:

- **My Books** — the canonical home, broad book collection, and sole browse
  surface for synced catalog metadata;
- **Reading** — the current Book and the To Read candidate chooser. Older
  Reading Journey and Primary Goal terms are not used on active screens;
- **Vocabulary** — the known-vocabulary import workflow and its durable status.
- **Catalogs** — learner-owned catalogue connections and metadata sync.

Catalogs is a configuration and sync destination, not a book-browse surface.
My Books remains the sole browse surface. Marking a Book To Read expresses the
learner's reading intent and may trigger acquisition and analysis. The upstream
catalog browser is retired.

Primary Goal is never a separate top-level destination. Analysis jobs, deck
preparation, and historical artifact provenance are supporting surfaces.
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
    move Books into or out of To Read

Reading (active study language)
    current Book, when present
    ordered To Read books
    Book anchors for current evidence and recovery
        focused deck-preparation task
        exact-analysis compatibility redirect to the Book anchor
    current, after-current-reading, and on-arrival coverage
    finish current reading
        choose what to read next -> Reading chooser (`/reading`)

Catalogs
    connection setup and sync status

Vocabulary
    known-vocabulary import and status

Secondary history
    operational analysis history
    reading, preparation, and vocabulary-transition history
```

### Current route compatibility

The shipped application uses `/library` for My Books and `/reading` for Reading.
`/` redirects to `/library`. `/journey` is a safe-GET compatibility alias to
`/reading`; obsolete mutation forms return `410 Gone` and do not mutate state. A
former `/journey/{bookID}` URL is a compatibility bookmark: after the same owner,
language, membership, and current-evidence checks, it redirects to the
corresponding `/reading` Book anchor and includes the Book's language as an
explicit one-request `?language=` selector. That selector is validated against
the learner's current study languages and does not change the stored active
language. Invalid or non-study values fall back to the active language.
Historical artifacts remain supporting records.
The application does not present a duplicate Book-detail, analysis-result, queue,
campaign, or plan surface.

The authenticated shell therefore exposes exactly My Books, Reading,
Vocabulary, and Catalogs in the top navigation. `/jobs` remains a supporting
surface reached from direct routes, not a navigation destination. Historical My Library, Learning,
queue, and learner-facing Campaign labels are not active navigation concepts;
compatibility aliases and operational terminology remain only where required by
existing routes, records, or infrastructure.

Existing nested analysis and artifact routes remain supporting routes. The
run-specific analysis route is a compatibility redirect rather than a separate
surface:

```text
/journey/{bookID} (compatibility bookmark to the Reading Book anchor)
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

The shipped [Book Covers](../features/book-covers.md) experience makes My Books
answer questions in this order:

1. What literature do I care about or want to find again?
2. Which Book is this, by cover, title, and author?
3. Is this Book To Read?
4. What can I do with this Book next?

The responsive collection grid lets a catalog-supplied cover lead visually while
keeping title and author visible beneath it. The language tag is carried by the
active-language heading, not repeated per item. Search remains scoped to the
active language; filtering and sorting support finding Books but do not turn
readiness into the default ranking. The cover is optional supporting metadata,
never the sole Book identity or an implicit mutation control.

Reading owns the current Book, To Read ordering, current evidence, coverage, and
recovery actions. My Books communicates whether a Book is To Read and keeps its
explicit disposition, metadata-refresh, and removal actions. Books without a
reachable Reading anchor remain operable without a detail page. Exact analysis
history and provenance are operational facts available through `/jobs`, not
sections on a generic learner-facing Book page. The grid remains moderately
dense and does not establish a generic large-card pattern.

The current My Books grid presents cover, title, author, To Read membership, and
explicit actions. It does not present evidence or current-reading state; those
remain on the Reading anchor.

## Reading information hierarchy

Reading answers questions in this order:

1. Which Book am I reading now, if any?
2. Which Books might I read next?
3. Which order is mine and how can I change it?
4. What current, after-current-reading, and on-arrival coverage evidence is
   trustworthy?
5. What happens to my modeled vocabulary when I finish the current Book?

Current reading is visually distinct but remains book-led rather than a large
metric card. To Read books are provisional and directly reorderable with
keyboard-operable controls. Drag may enhance but never replace **Move earlier**
and **Move later**.

The learner's order is always the only active order. Current,
after-current-reading, and on-arrival coverage labels must be available,
including a lower-bound label when an earlier Book cannot contribute
trustworthy modeled vocabulary. Internal threshold and top-unknown data remain
available to analysis and forecast
services, but the learner-facing overview leads with books and the consequence
of the learner's order.

Unassessed or otherwise untrustworthy books stay in To Read at the learner's
chosen position. Mouseion explains the evidence gap, gives no fabricated
coverage, and labels downstream projections as lower bounds when an earlier
contribution is unavailable rather than moving or demoting books.

## Finishing current reading and choosing what to read next

Reading completion and artifact readiness are independent facts. Finishing the
current Book records the reading fact and accepts its frozen snapshot into
modeled Known vocabulary; it then removes the Book from To Read and clears
current reading. The Book remains in My Books, history, and provenance. No next
Book is started automatically. ADR 0072 defines the atomic transition and
forecast semantics.

The restrained receipt proceeds in three beats:

1. identify the Book and state **Reading finished**;
2. show the exact newly-Known and already-Known counts, including zero for an
   empty snapshot;
3. link **Choose what to read next** to the candidate chooser at `/reading`.

Two required branches are:

- **Current Book finished with a non-empty snapshot.** State the reading outcome
  and exact newly-Known and already-Known counts.
- **Current Book finished with an empty snapshot.** State the reading outcome and
  show zero newly-Known and already-Known identities.

The receipt does not replay forecasts or choose the next Book. The candidate
chooser presents eligible To Read books without an optimality claim; its empty
state returns calmly to My Books. **Read again** in Read history returns the
finished Book to To Read, and a later start freezes a fresh snapshot.

## Analysis continuity

The Reading page's Book anchor is the canonical presentation for a To Read
Book's identity, relationship, current evidence, forecast, and recovery actions.
The run-specific route `/books/{book-id}/analyses/{analysis-run-id}` remains only
as a compatibility redirect to the anchor for reachable members and returns 404
otherwise, preserving valid deep links without rendering a second insight
surface. Internal threshold and top-unknown analysis data remain durable even
though their learner-facing presentation is retired.

Deck preparation is a separate focused task at
`/reading/books/{bookID}/deck/preparations/new`. It is bound to the exact current
analysis and returns to the originating Reading Book anchor; it does not replace
Book identity, start current reading, or mark vocabulary known.

The operational job page remains responsible for queued/running progress,
cancellation, retry, attempts, and failure recovery. When work completes, its
primary action is **View in Reading**, which opens the canonical Book
anchor directly or through the compatibility redirect. `GET /jobs` remains the
operational history surface for current and prior runs.

## Active study language, study languages, and Vocabulary ownership

Study languages are derived from the distinct normalized language tags of the
learner's active chosen-language Books. For catalog-synced Books, the
catalog entry is the source of truth and connection re-sync is the only way
the language changes; there is no separate Settings selection to maintain.

The **active study language** is a stored selection pointing into that derived
set — context, not configuration: it chooses which study language the
language-scoped surfaces (My Books browse and search, Reading,
Vocabulary) present, and never defines which languages are studied. It defaults
deterministically (the sole study language, else the language of the most
recently activated chosen-language Book) and resets lazily when the selection
leaves the set. A shell-level switcher carries it on every authenticated screen;
changing it navigates to the same screen in the new language on language-scoped
screens. Compatibility bookmarks validate a Book's own language before redirecting
to its Reading anchor. The URL's one-request language selector controls that
Reading view but never changes the stored active mode. A newly arrived study
language appears passively
in the switcher (marked "new") without changing the mode.

Vocabulary owns the additive known-vocabulary import workflow, scoped to the
active language; import is always eligible there. The page presents import
status and result summaries but does not display the known-vocabulary read model.
Import eligibility remains limited to the derived study-language set, and
known-vocabulary-only languages stay selectable in the switcher as read-only
"no books" entries. Catalog
metadata changes do not delete known-vocabulary rows, books, analyses, decks, or
artifact provenance. Journey and Goal relationships remain independent of
vocabulary import; their shipped consequences are defined by ADR 0034, ADR 0072,
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
2. **Primary Goal, snapshot, completion, and forecast** follow [ADR 0072: Goal-owned
   vocabulary snapshots and sequential Reading Journey forecast](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md),
   tightened by [ADR 0049: Reading intent triggers analysis](../adr/0049-reading-intent-triggers-analysis.md): one
   Primary Goal is promoted only from a Journey member with successfully
   completed current analysis; it owns the immutable snapshot and clears on
   completion or explicit change/clear.
3. **Campaign queue retirement and historical artifacts** follow [ADR 0056](../adr/0056-retire-campaign-learner-surface.md):
   the Journey is the only learner-facing plan, while old campaign and deck
   records remain supporting provenance.
4. **Generated vocabulary** follows ADR 0072: generated rows are immutable
   provenance, not a current selection exclusion or knowledge claim.
5. **Per-book analysis evidence** retains the individual threshold and unknown
   vocabulary contract; the Journey overview uses only the ADR 0072 forecast.
7. **Routes and terminology** are reconciled in the shipped shell and supporting
    surfaces: My Books, Reading, Vocabulary, and Catalogs are the active
    navigation destinations; `/known-vocab`
   remains a compatibility route with its documented redirect. Campaign routes
   are not learner-facing webapp routes.
8. **One current analysis per Book** follows [ADR 0040](../adr/0040-one-current-analysis-per-book.md):
     Reading Book anchors are the current evidence surface for To Read members, prior
    runs remain operational audit records, and run-specific result URLs redirect
    to the applicable Book anchor.
9. **Catalog sync** follows the accepted contract in
   [ADR 0041](../adr/0041-catalog-sync-metadata-first.md), with its language
   scope reconciled by [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
   each learner-owned connection periodically reconciles metadata for every
   non-English language whose NLP pipeline is ready, without downloading EPUB
   content, deleting local state, or invalidating scope or analysis. ADR 0077
   adds independent optional cover-image retrieval without changing that EPUB
   boundary.
10. **Language view retirement** follows [ADR 0057](../adr/0057-retire-language-view-panel.md):
    the panel proposed by ADR 0042 has no current route or screen contract.
     Per-Book evidence remains on Reading Book anchors in the shipped cover
      experience; the My Books grid does not duplicate it.
     No replacement aggregate is implied.
11. **Derived study languages and Vocabulary** are resolved by
    [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
    chosen-language Books define the language set, Vocabulary owns
    known-vocabulary import, and Settings is removed from primary navigation.
    Any future change to the derived-language source or Vocabulary's destination
    must return to this checkpoint.
12. **Active study language mode** is resolved by
    [ADR 0050](../adr/0050-active-study-language.md): one stored selection
    pointing into the derived set scopes My Books, Reading, and
    Vocabulary; `?language=` params, the "All languages" pill, and per-row
    language tags are removed; a shell-level native switcher carries the mode;
    new languages arrive passively; legacy no-language Books surface only
    through a **needs language** strip.
13. **Per-language Journey and Goal** are resolved by
    [ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md): Reading
    Journey and Primary Goal identity are (owner, study language) with
     per-language revisions; the ADR 0072 forecast is language-correct by
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
