# Information architecture

> **Study cutover.** [ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md) merged the Reading page into My Books, moved Book-scoped vocabulary Browse to the Book detail page (`/books/{id}`), retired the Prepared deck and Known vocabulary, and made [Study](../features/study.md) the only vocabulary workflow. The shell's destinations are My Books, Vocabulary and Catalog, with Study at `/{language}/study`. Where this document names a Reading page or destination, a working desk, a prepared deck or Known vocabulary, read it as the retired design; the current contract is in the [Reading workflow](../features/reading-workflow.md).

Status: **Canonical shipped route and navigation contract, with an explicitly
marked accepted Vocabulary target below.** The Reading
workflow is implemented without Journey ordering, sequential forecasts, or
Primary Goal. ADR 0074 assigns deck work to the focused preparation task. ADR
0072 remains historical context for retired snapshot/forecast behavior. This document follows
the one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent analysis trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md), with the
standalone learner action retired by [ADR 0054](../adr/0054-retire-standalone-analysis-action.md).
Language is the
app's organizing context: [ADR 0131](../adr/0131-the-url-carries-the-study-language.md)
carries the study language in the address of every language-dependent surface
(superseding the stored mode of [ADR 0050](../adr/0050-active-study-language.md)). Current reading
is scoped by study language, and catalog maintenance is a fourth
principal destination per
[ADR 0058](../adr/0058-catalog-maintenance-principal-destination.md). ADRs
continue to own persistence and historical decision details.

Mouseion is organized around literature the learner cares about and the next
reading decision. It foregrounds books, bibliographic identity, learner
intention, and independent reading history rather than backend jobs,
preparation mechanics, or a generic dashboard.

## Learner goals

The product supports these top-level goals:

1. keep a broad personal collection of books Mouseion knows about;
2. work in one active study language at a time;
3. mark a To Read Book as previously read, read a Read Book again, and remove
   a Book from My Books;
4. choose, end, or switch the one current Book for the active language;
5. review current evidence bands for To Read Books without a suggested order;
6. finish a Book and retain its Read history independently from disposition;
7. understand derived study languages and investigate vocabulary;
8. maintain the learner-owned catalog connections that feed My Books.

**Accepted target (partially shipped):** on Reading, the current Book's
vocabulary Browse lists effective identities in the active language's Current
reading Book, ranked by occurrence count after excluding already accounted-for
vocabulary by default. Concordance remains cross-Book in Vocabulary; saved
selections and Custom decks are retired. See the
[Vocabulary feature specification](../features/vocabulary-browse-and-concordance.md)
and [Reading-owned Book vocabulary](../archive/features/reading-owned-book-vocabulary.md).

The recurring experience rhythm is:

```text
Catalog -> To Read -> Reading chooser -> Current reading
                              <- Read history <- Finish <-
```

This rhythm does not imply a required pipeline for every Book. Catalog browsing
and sync, Add, analysis, current-reading selection, reading completion, and
prepared-deck artifact work remain distinct transitions. Sync keeps each
connection's Catalog index current and follows title, author and cover changes
onto Books the learner already chose; it never creates a Book or membership.
Choosing a Book (Add) expresses reading intent and acquires the current EPUB as
part of becoming To Read. Starting current reading freezes the vocabulary
snapshot; finishing accepts eligible identities into modeled Known vocabulary
and creates an independent completion record.

## Principal learner-facing model

### My Books

**My Books** is the broad collection of books Mouseion knows about for the
learner. It can include:

- acquired and metadata-only books;
- assessed and unassessed books;
- Books the learner chose, in To Read or Read;
- currently read and previously read books;
- completed books;
- books with stale, questionable, incomplete, or unavailable evidence.

My Books is a searchable bibliographic catalog, not a readiness ranking or a
list of obligations. Title, author, edition when relevant, and learner intent
precede analysis status. Processing state appears only to explain available
evidence or the next relevant action.

My Books is browsed within the active study language: browse, paging, and search
are scoped to it, and no "All languages" default exists. It has two tabs, **To
Read** (including the current Book) and **Read**. A Book without a chosen
language is not listed in My Books. For a fresh account, its empty state leads to
`/catalogs`; catalogue setup and sync maintenance are owned by the Catalog
destination and its Manage sources page. My Books holds only Books the learner
chose from the Catalog; sync maintains each connection's local Catalog index
instead of adding Books.

### Reading

**Reading** at `/{language}/reading` owns the current Book for the page's study language or,
between Books, an unordered chooser of To Read candidates in neutral title order.
Every To Read Book is its own entry. While a Book is current, the same page hosts its full vocabulary Browse
(the working desk); other To Read candidates are not listed. Coverage bands label
current evidence and do not rank candidates or predict outcomes. Reading shows no
"Book N of M" line and nothing points to a next Book. Starting freezes a
vocabulary snapshot; finishing records a completion without selecting the next
Book. End and switch are explicit reversible actions.

There is at most one current Book per owner and study language. The Book's
My Books disposition remains separate from current-reading state and Read
history. **Read again** returns a previously completed Book to To Read while
preserving every earlier completion. See the shipped
[Reading workflow](../features/reading-workflow.md). Journey ordering and
Primary Goal terminology in older ADRs is historical only.

## Supporting product objects

These objects remain important, but they do not define principal navigation:

- **Book** — owner-scoped learner-facing bibliographic identity and the center
  of My Books membership, reading, analysis, and current-reading relationships. It may
  exist without acquired content; ADR 0035 defines its identity and lifecycle.
- **Source snapshot** — immutable acquired EPUB content and extracted units;
  provenance rather than a destination. It is linked after acquisition and is
  never a metadata-only placeholder.
- **Reviewed scope** — immutable learner-confirmed source-unit selection. Scope
  review presents reliable top-level EPUB 3 TOC entries as an all-on checklist,
  or readable persisted units in flat spine order when projection is
  unreliable; TOC entries expand to existing unit IDs.
- **Analysis run** — asynchronous analysis run; queue and retry details are
  operational state.
- **Current analysis** — the one completed analysis whose evidence supports the
  Book anchor in Reading. Its immutable corpus and provenance remain
  backend facts.
- **Prepared deck** — APKG artifact from the Current reading's frozen Book
  vocabulary snapshot and exact analysis, prepared through the focused deck task
  (accepted target; historical pre-reading artifacts remain).
- **Reading vocabulary snapshot** — the immutable Book vocabulary candidate set
  frozen when current reading starts, with its selection and analysis provenance.
- **Reserved vocabulary** — the current-reading snapshot projected into selection
  state for that study language; it is not a separate study workflow.
- **Known vocabulary** — owner-scoped vocabulary accepted when a current
  reading is finished (ADR 0092 retired import).
- **Catalog connection** — learner-owned OPDS endpoint and credentials.
- **Catalog sync** — periodic metadata reconciliation for one learner-owned
  connection. It maintains the connection's Catalog index and follows title,
  author and cover changes onto Books already linked by catalog-entry alias; it
  never creates a Book, membership, or disposition. Content is trusted
  immutable for sync, so it never invalidates scope or analysis. The metadata
  contract is defined by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md)
  and its language scope is reconciled by ADR 0043.
- **Study language** — a distinct normalized language tag of the owner's active
  chosen-language Books; it defines known-vocabulary language context and is
  derived rather than selected in Settings.
- **Active study language** — the study language of the per-language page in
  view, named by its address ([ADR 0131](../adr/0131-the-url-carries-the-study-language.md)).
  The **Last study language** is the one most recently viewed (or the language of
  a Book just opened from the Catalog); it only resolves addresses and links
  without a language.
- **Language lens** — historical terminology for the retired panel proposed by
  [ADR 0042](../adr/0042-derived-language-corpus-view.md). It is not a current
  object or learner-facing surface; [ADR 0057](../adr/0057-retire-language-view-panel.md)
  records the retirement.

A relationship graph, not a strict containment hierarchy, connects these
objects. Every Book in My Books is Currently reading, To Read or Read. Current
reading, workflow disposition, and Read history are independent facts. Earlier
analysis runs remain addressable as operational audit records after vocabulary,
source, or scope changes; they are not parallel learner result surfaces.

## Primary navigation

The authenticated shell exposes four principal destinations:

- **My Books** — the canonical home for the Books the learner chose, with
  search, reading intent, and reading history;
- **Reading** — the current Book with its vocabulary Browse, or the To Read
  candidate chooser between Books. Older Reading Journey and Primary Goal terms
  are not used on active screens;
- **Vocabulary** — Concordance; the accepted target removes saved selection
  and Custom deck UI beneath this destination.
- **Catalog** — the built-in Classical catalog and every OPDS source's local
  Catalog index as ordinary card sections, and the sync status of each
  learner-owned source.

Catalog keeps the built-in, read-only **Classical catalog** as an ordinary
source section beside each OPDS source, with a work page for each Work. Every
source shows one kind of card (image, title, author, one detail line, ownership,
**Add** and **I've read this**) in a grid ordered by title then author and paged
12 at a time; the image is the entry's retained cover or a **Title plate**. One
search and one language filter cover every source, and results are grouped by
source. The Classical section appears when the filter is Ancient Greek or all
languages; the old Classical index and author-detail URLs permanently redirect
to the Catalog filtered to Ancient Greek. The Language filter is the Catalog's
only language control: it always offers Ancient Greek, and the shell language
picker is not shown on the Catalog. The language filter defaults to the Last study language, or to
all languages when the learner has none. At the top of the page it shows one
status line per OPDS connection — never synced, syncing, last synced or sync
failed — with **Sync now**; a failed line names the connection and links to
**Manage sources**.
Connection setup, credentials, editing, deletion, sync and failure recovery live
on **Manage sources** at `/catalogs/sources`. The Catalog is not a fifth
destination. My Books remains the sole **bibliographic Book** browse surface;
the Catalog browses entries, and the Browse working desk on Reading lists
vocabulary identities, not Books or catalog entries. Marking a Book To Read
expresses the learner's reading intent and may trigger acquisition and analysis;
Add on a Classical card creates or restores its Book as To Read and starts
the same download and preparation. The upstream catalog browser is retired.

Analysis jobs, deck preparation, and historical artifact provenance are
supporting surfaces.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shell also carries a native **active study language** control after the four
destinations in document and keyboard order. Wide screens place it at the
trailing end; compact screens show it beside the wordmark above the destinations.
Changing it navigates to the same screen in the new language on language-scoped
screens and updates the stored mode elsewhere; it lists the derived study
languages plus any known-vocabulary-only language marked "no books", marks a
newly arrived study language "new", and never auto-switches on navigation or
sync.

There is no separate **Catalogues / Browse** sub-navigation. `/catalogs` is the
Catalog, `/catalogs/sources` is Manage sources, and `/library` remains the
canonical local browse surface.

## Route and screen hierarchy

The hierarchy below records shipped conceptual and route ownership. It does not
replace the ADRs' detailed storage and compatibility decisions.

```text
Authentication
    sign in or first-account onboarding

My Books
    To Read and Read tabs
    move Books into or out of To Read

Reading (active study language)
    current Book, when present
    current Book's vocabulary Browse (working desk)
    between Books: unordered To Read candidates in neutral title order,
        one ordinary entry per Book
    Book anchors for current evidence and recovery
        focused deck-preparation task
        exact-analysis compatibility redirect to the Book anchor
    current evidence only; no sequential forecast
    finish current reading
        choose what to read next -> Reading chooser (`/reading`)

Catalog
    one search and one language filter over every source
    Classical section (Ancient Greek or all languages): paged card grid, one
        card per Reading unit
    work page (`/catalogs/classical/works/{work-key}`)
    retired `/catalogs/classical` and author-detail URLs permanently redirect to
        `/catalogs?language=grc`
    each OPDS source section: paged card grid of local Catalog index entries
    In My Books (`/catalogs/books/{book-id}/in-my-books`)
    one status line per OPDS source with Sync now
    Manage sources (`/catalogs/sources`): connection setup, credentials,
        editing, deletion, sync, and failure recovery

Study (active study language)
    Today: due cards, the day's New-word batch, Fluency
    Report on a revealed card: Wrong word or Unsuitable context
        Unsuitable context -> the reported card with Undo and Next
    Reported words (`/{language}/study/reports`): open Wrong-word reports
        Review in Lemma review (opens the reported occurrence)
        Dismiss

Vocabulary (partially shipped)
    Concordance exact lookup and KWIC context (shipped landing slice)
    focused sentence study and grammar exploration (shipped)

Secondary history
    operational Book preparations and Catalog sync history (`/jobs`)
    reading, preparation, and vocabulary-transition history
```

### Current route compatibility

The shipped application addresses My Books, Reading, and Vocabulary under a
language prefix (`/de/library`, `/de/reading`, `/de/vocabulary`). The unprefixed
`/library`, `/reading`, and `/vocabulary` are compatibility redirects to the
prefixed form (the query's valid `?language=`, else the Last study language).
`/` redirects to My Books. `/journey` is a safe-GET compatibility alias to
`/reading`; obsolete mutation forms return `410 Gone` and do not mutate state. A
former `/journey/{bookID}` URL is a compatibility bookmark: after the same owner,
language, membership, and current-evidence checks, it redirects to the
corresponding Book anchor in Reading. The bookmark is Book-scoped: it redirects
under the Book's own language prefix (`/{book language}/reading`), whatever the
Last study language is (ADR 0131).
Historical artifacts remain supporting records.
The application does not present a duplicate Book-detail, analysis-result, queue,
campaign, or plan surface.

The authenticated shell therefore exposes exactly My Books, Reading,
Vocabulary, and Catalog in the top navigation. `/jobs` remains a supporting
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
/vocabulary (Concordance)
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

Reading owns the current Book or unordered To Read chooser, current evidence, and
recovery actions. My Books communicates whether a Book is To Read and keeps its
explicit disposition and removal actions. Books without a
reachable Reading anchor remain operable without a detail page. Exact analysis
history and provenance are operational facts available through `/jobs`, not
sections on a generic learner-facing Book page. The grid remains moderately
dense and does not establish a generic large-card pattern.

The current My Books grid presents cover, title, author, disposition, and
explicit actions. Every Book is one ordinary row, classical or not: a classical
Book adds its Work title, Edition label, position and Classical catalog
provenance, and two Editions of one Work are two rows told apart by their
Edition labels. Each row carries the ordinary per-Book Analysis update notice;
there is no Work-level move, removal or update. Filters count Books.
My Books does not present analysis evidence or current-reading state; those
remain on the Reading anchor.

## Reading information hierarchy

Reading answers questions in this order:

1. Which Book am I reading now, if any?
2. Which Books might I read next?
3. What current coverage evidence is available for each candidate?
4. What happens to my modeled vocabulary when I finish the current Book?

Current reading is visually distinct but remains book-led rather than a large
metric card. The between-Books chooser lists candidates in neutral title order
with one ordinary entry per Book; incomplete evidence stays distinct in that
Book's margin, and each Book carries the ordinary per-Book Analysis update
notice. No learner order,
after-reading projection, forecast,
next-Book pointer, or recommendation is presented. Unassessed or otherwise
untrustworthy Books remain visible with an explicit evidence gap and no
fabricated coverage.

## Finishing current reading and choosing what to read next

Reading completion and artifact readiness are independent facts. Finishing the
current Book records the reading fact and accepts eligible identities from its
frozen snapshot into modeled Known vocabulary; it then moves the Book to Set
Aside and clears current reading. The Book remains in My Books and Read history.
No next Book is started automatically.

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

The Reading page's Book anchor is the canonical presentation for a Book's
identity, current-reading state, current evidence, and recovery actions.
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
learner's active chosen-language Books. For a Book linked to a Catalog entry,
the entry supplies title, author and cover at each sync, while the Book keeps
the language its Add chose; there is no per-Book metadata refresh and no
separate Settings selection to maintain.

The **active study language** is the language named by the address of the page
in view (ADR 0131), drawn from that derived set — context, not configuration:
it chooses which study language the language-scoped surfaces (My Books browse
and search, Reading, Vocabulary) present, and never defines which languages are
studied. The **Last study language** is remembered only to resolve unprefixed
addresses and navigation links; it is set when a per-language page renders (or
a Book is opened from the Catalog), defaults deterministically (the sole study
language, else the language of the most recently activated chosen-language
Book), and resets lazily when it leaves the set. A shell-level switcher on
per-language pages navigates to the same kind of page in another language with
screen, filter, and return state reset. A newly arrived study
language appears passively in the picker (marked "new"); the Last study
language changes only when a prefixed page renders.

Vocabulary owns Concordance, scoped to the active language; known-vocabulary
import is retired (ADR 0092). Browse
lives on Reading for the current Book; it lists current, owner-specific effective
vocabulary evidence and distinguishes analysis coverage from an empty or filtered
inventory. It does not display Known-only identities without current occurrences.
Known-vocabulary-only languages stay selectable in the switcher as read-only
"no books" entries. Catalog metadata changes do not delete known-vocabulary
rows, books, analyses, decks, or artifact provenance. Reading and Book Prepared
deck behavior remains independent of Browse.

### Accepted Vocabulary target (partially shipped)

The [Vocabulary feature specification](../features/vocabulary-browse-and-concordance.md)
and [ADR 0085](../adr/0085-reading-owned-book-vocabulary.md) define the
accepted target. Keep exactly the existing four primary destinations;
Vocabulary is **Concordance** at `/vocabulary`, with no peer-view navigation.
Browse is hosted on Reading
(`/reading`) as the working desk for the active study language's Current reading
Book. Browse covers effective
vocabulary identities in that Book, not the Book catalog or only its
prepared-deck candidates. Old Browse query state on `/vocabulary` is not
reinterpreted as lookup. Browse identity links
and exact surface/effective/analyzer lookup with paged KWIC context are shipped.
Applied multi-Book and grammar filters plus focused, morphology-first sentence
study are shipped. Saved Browse selection and Custom decks are shipped but are
retired by the accepted Reading-centered target. Concordance exposes sentence
context without altering analyzer evidence. Return paths among the Vocabulary
views and from focused sentence study preserve context. Reading's only Vocabulary surface is the
current Book's Browse; it adds no permanent Vocabulary navigation or new
study-state transition. Known import retains its existing
contract rather than merging with Browse inventory or presenting a Known-only
vocabulary list.

The hierarchy above describes both shipped and target behavior; present the
implemented Concordance lookup/KWIC, grammar exploration, and sentence-study
interactions as available, not the retired Custom-deck workflows. My Books
remains the only bibliographic browse surface.

<a id="contract-changes-requiring-planneradr-work"></a>

## Architecture decisions and target reconciliation

The following decisions record shipped architecture. Accepted ADRs remain
authoritative for persistence, historical records, and compatibility details.

Broader My Books membership is resolved by
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md): an
owner-scoped bibliographic Book and its My Books membership are distinct from
immutable acquired source evidence. Choosing a Catalog entry creates My Books
membership; moving a Book to To Read acquires validated source evidence when
needed. Sync only maintains the Catalog index and follows metadata onto linked
Books.

1. **Retired Journey ordering** is preserved in [ADR 0034](../adr/0034-reading-journey-identity-ordering.md)
   as historical architecture context. It is not the shipped Reading model.
2. **Retired Primary Goal and sequential forecast** are preserved in [ADR 0072](../adr/0072-goal-owned-vocabulary-and-journey-forecast.md).
   Current reading freezes its own vocabulary snapshot and completion history;
   no ordered Journey forecast is shown.
3. **Campaign queue retirement and historical artifacts** follow [ADR 0056](../adr/0056-retire-campaign-learner-surface.md):
   old campaign and deck records remain supporting provenance, not active plans.
4. **Generated vocabulary** remains immutable
   provenance, not a current selection exclusion or knowledge claim.
5. **Per-book analysis evidence** retains the individual threshold and unknown
   vocabulary contract; the Reading chooser uses current coverage bands only,
   grouping a Work's To Read Books into one entry rather than ranking them.
6. **Routes and terminology** are reconciled in the shipped shell and supporting
    surfaces: My Books, Reading, Vocabulary, and Catalog are the active
    navigation destinations; `/known-vocab`
   is retired and returns 404 without a redirect. Campaign routes
   are not learner-facing webapp routes.
7. **One current analysis per Book** follows [ADR 0040](../adr/0040-one-current-analysis-per-book.md):
     Reading Book anchors are the current evidence surface for To Read members, prior
    runs remain operational audit records, and run-specific result URLs redirect
    to the applicable Book anchor.
8. **Catalog sync** follows the accepted contract in
   [ADR 0041](../adr/0041-catalog-sync-metadata-first.md), with its language
   scope reconciled by [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
   each learner-owned connection periodically maintains its Catalog index for
   every non-English language whose NLP pipeline is ready and follows metadata
   onto Books already linked to an entry, without creating Books, downloading
   EPUB content, deleting local state, or invalidating scope or analysis. ADR 0077
   adds independent optional cover-image retrieval without changing that EPUB
   boundary.
9. **Language view retirement** follows [ADR 0057](../adr/0057-retire-language-view-panel.md):
    the panel proposed by ADR 0042 has no current route or screen contract.
     Per-Book evidence remains on Reading Book anchors in the shipped cover
      experience; the My Books grid does not duplicate it.
     No replacement aggregate is implied.
10. **Derived study languages and Vocabulary** are resolved by
    [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md):
    chosen-language Books define the language set, Vocabulary owns
    Concordance (ADR 0092 retired known-vocabulary import), and Settings is
    removed from primary navigation.
    Any future change to the derived-language source or Vocabulary's destination
    must return to this checkpoint.
11. **The study language** is carried by the address, per
    [ADR 0131](../adr/0131-the-url-carries-the-study-language.md): My Books,
    Reading, and Vocabulary live under `/{language}/…`, and the Last study
    language only resolves unprefixed addresses and navigation links. There is
    no one-request language selector. The "All languages" pill and per-row
    language tags are removed; a shell-level native switcher navigates between languages;
    new languages arrive passively; Books without a chosen language are not
    listed in My Books.
12. **Historical per-language Journey and Goal** are recorded by
     [ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md). The
     shipped Reading state is independently scoped by owner and study language;
     it has no Journey order or Goal identity.

## Cross-linking rules

Every principal or nested screen makes clear:

1. which Book, Reading state, analysis, or setting is in context;
2. which facts are current and which are conditional;
3. the learner's available next decision and why it is available;
4. how to return to the parent context without reconstructing the route through
   global navigation.

Operational identifiers, attempt counts, and historical provenance remain
available where useful, but they must not displace book title, author, learner
intention, or the next meaningful choice. Classifier and recommendation data
are compatibility history, not current scope-review concepts.
