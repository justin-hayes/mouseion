# Reading Collection Browsing

Status: Implemented, including the cover-grid extension · Date: 2026-09-02 · Updated: 2026-11-22

## Motivation

Reading is the learner's broad bibliographic collection, not a queue or a
readiness ranking. As metadata-first catalog sync increases its size, a single
unpaged list is no longer sufficient for finding a title, author, edition, or
language. The canonical information architecture already promises a searchable
bibliographic catalog; this feature makes that promise concrete.

## Goal

Let learners find and open a Book in a large local Reading collection through
paging and text search scoped to the active study language, while keeping
bibliographic identity and learner intention ahead of analysis evidence. My
Books is always browsed within the active study language; there is no
cross-language "All languages" default ([ADR 0050](../adr/0050-active-study-language.md)).

## Scope

This feature applies to Reading at `/{language}/reading`. It defines collection controls,
collection hierarchy, progressive enhancement, and accessibility. Reading is the
sole browse surface for Books; the Catalog at `/catalogs` separately browses the
Classical catalog and each connection's local Catalog index. There is no live
OPDS browse or search surface.

## Shipped cover-grid extension

[Book Covers](book-covers.md) defines the shipped presentation for the repeated
active-language results: one responsive cover grid with title and author always
visible, workflow disposition as its relationship signal, and labeled To Read
plus secondary actions. It preserves this feature's search, deterministic
ordering, paging, empty states, language scope, and progressive-enhancement
contracts. Reading owns current evidence and current-reading presentation.

## Requirements

### Active-language scope

- Reading always presents the active study language's Books. Browse, paging,
  and search are scoped to it; the shell's active-language switcher changes the
  scope instead of a per-page control.
- Reading holds only chosen Books: every Book is Currently reading, To Read or
  Read. Books without a chosen language belong to no language partition and are
  not listed; one with reading history keeps its membership and is reported by
  the backfill (see [Upgrading existing accounts](#upgrading-existing-accounts)).
- The active language is context, not a finding aid: it is carried by the page
  heading and the switcher, not repeated per row.

### Scoped collection search and paging

- One text search filters the active study language's Reading collection,
  including metadata-only and acquired Books. It is explicitly not a live OPDS
  query and does not depend on an upstream connection being available.
- Search combines predictably with the active language and paging controls.
- Paging uses stable, deterministic ordering and preserves the current query in
  real links and forms.
- Empty-collection, search-empty, and no-language-results states remain distinct
  and offer an appropriate way to clear a filter, revise a query, or start
  catalogue setup through the `/catalogs` destination.

### Collection hierarchy and book selection

- Items lead with the optional Book cover, followed by the full title and author;
  the language tag is carried by the active-language heading, not repeated per
  item.
- The collection has two tabs: **To Read** (including the current Book, with its
  label) and **Read**. The Inbox tab and filter, and the separate Read-history
  filter, are retired.
- Workflow disposition is the relationship signal. Evidence and current-reading
  state belong to Reading and never turn the collection into
  a metric-led dashboard.
- Missing author or edition/year is stated or omitted without inventing
  metadata.
- Choosing a Book opens its detail page at
  `/books/{bookID}`. Metadata-only and otherwise incomplete Books remain
  operable from their rows through disposition and catalog actions.
- The list does not expose batch-select-then-analyze behavior.

### Removing from Reading

- **Remove from Reading** is the opposite of Catalog Add and belongs to every
  Book's More actions. It ends Reading membership and changes nothing else:
  Book identity, analysis, corrections, exclusions, completions and Known
  vocabulary all remain.
- The current reading is never removed. Its row states
  why the action is unavailable, and a direct request is refused with the same
  reason instead of ending the reading.
- The Catalog offers **Add** for an entry whose Book left Reading. Adding it
  again restores the same Book with its evidence, for both an OPDS Book and a
  classical Book.

### Classical works

- A classical Book is an ordinary row with its own actions. The row identifies
  the Book by its Work title, Edition label, Work position such as "Histories,
  Book 1" and "Book 1 of 9" for a leftover per-book Book (a whole Work has no
  position), Greek title, and Classical catalog provenance.
- Two Editions of one Work are two rows, told apart by their Edition labels.
- A leftover Book on a withdrawn per-book definition stays an ordinary row. The
  learner removes it with **Remove from Reading**, one Book at a time.
- Each Book carries the ordinary per-Book Analysis update notice; the current
  Reading's Book asks to end the Reading first.
- Search matches title, author, English title, or Greek title ignoring accents.
  A trailing number is ordinary search text, not a Book number. Greek title
  matching uses the Go search key, never SQL `lower()`.

### Progressive enhancement and accessibility

- The baseline is a complete server-rendered page using native links, forms,
  headings, lists or tables, and pagination controls.
- HTMX may update the results region, counts, and paging without replacing the
  browser's usable URL or making JavaScript mandatory.
- Filters have persistent labels and expose their selected state in text and
  native semantics; color and pill shape are not the only indication.
- Dynamic result updates use one scoped live region, preserve or deliberately
  restore focus, and do not announce every row.
- Keyboard order follows search, language controls, results, and paging. Every
  book and page is reachable without pointer gestures.
- Narrow layouts preserve title, author, edition/year, and the book link without
  page-level horizontal scrolling. Long titles and names wrap rather than being
  truncated into ambiguity.

## States

| State | Required presentation | Primary action |
|---|---|---|
| Empty collection | Explain Reading and lead to the Catalog. | Browse the Catalog |
| Collection available | Show scoped search, a cover-led collection for the active language, and paging. | View in Reading on the current Book and To Read rows; More actions offer Mark as previously read (unstarted To Read), Read again (Read), or Remove |
| Cover unavailable or pending | Keep the title and author visible beside a truthful placeholder. | The same actions as with a cover |
| Search empty | Retain the query within the active language and state that the local collection has no match. | Revise or clear search |
| Later page becomes empty | Return to the nearest valid page without losing the query context. | Continue browsing |
| Enhancement failed | Keep or restore the ordinary server-rendered form/link path. | Submit normally |

## Upgrading existing accounts

Reading holds only chosen Books. A one-time data-only backfill removes the
membership of every Book that is active in Reading, is not To Read, has no
reading history and is not the current reading. That covers unread Inbox Books,
never-prepared classical Inbox Books and unread Books without a chosen
language. Other Books with reading history stay. A language-less Book with
history also leaves Reading, because Reading is browsed within a study
language and would list it nowhere: it behaves like any removed Book, keeps its
completions, and can be added again with a language. The backfill reports each
one.

The backfill is operator-owned. Run it once, after the release deploys and
after every catalog connection has completed at least one sync; it is
idempotent and retry-safe, but re-running it is not a rollback and restores
nothing, because membership is the only thing it changes. A removed Book is
restored by Catalog **Add**, which restores the same Book with its evidence as
long as its entry is in the connection's Catalog index — which the sync gate
is there to ensure. A classical Book is restored by adding its Reading unit
from the Classical catalog. The gate refuses to run until every connection's
Catalog index has completed a sync, so no entry becomes unfindable during the
upgrade. Run `MOUSEION_DATABASE_URL=... go run ./cmd/backfill`; the output
states how many memberships ended and names each language-less Book with history whose membership it ended.
The [operations guide](../operations.md#maintenance-commands) documents the
command's prerequisites, output, and recovery.

## Non-goals

- Live OPDS search, catalog discovery, or upstream pagination.
- Cross-language browse or search; an "All languages" default.
- Batch selection or batch analysis.
- A default readiness ranking, recommendation, or “best next book.”
- Changing Book identity, evidence, current reading, or reading history
  contracts.
- Requiring JavaScript, drag interactions, or a spatial collection canvas.

## Acceptance criteria

- Browse, search, and paging return only Books in the active study language.
- Reading shows exactly the To Read and Read tabs. The Inbox and needs-language
  states are retired: `/library?needs-language`, `?disposition=inbox`, and an
  unknown disposition return 404.
- Query and page combinations return stable owner-scoped results and preserve
  their state in navigable URLs.
- Search covers the active language's local collection and performs no OPDS
  request.
- Analyzed To Read items link to their canonical Reading anchor;
  other rows do not link to a detail page.
- Empty, no-match, and paging-boundary states are distinct.
- Remove on a To Read or Read Book ends membership only, and the Catalog then
  offers Add again; the current Book is refused with a reason.
- Add restores the same Book after Remove.
- The full workflow is keyboard-operable and usable without JavaScript; HTMX
  enhancement preserves focus, announcements, and URL meaning.
- Narrow and long-content cases retain bibliographic identity and usable actions.
