# My Books Collection Browsing

Status: Implemented, including the cover-grid extension · Date: 2026-09-02 · Updated: 2026-09-23

## Motivation

My Books is the learner's broad bibliographic collection, not a queue or a
readiness ranking. As metadata-first catalog sync increases its size, a single
unpaged list is no longer sufficient for finding a title, author, edition, or
language. The canonical information architecture already promises a searchable
bibliographic catalog; this feature makes that promise concrete.

## Goal

Let learners find and open a Book in a large local My Books collection through
paging and text search scoped to the active study language, while keeping
bibliographic identity and learner intention ahead of analysis evidence. My
Books is always browsed within the active study language; there is no
cross-language "All languages" default ([ADR 0050](../adr/0050-active-study-language.md)).

## Scope

This feature applies to My Books at `/library`. It defines collection controls,
collection hierarchy, progressive enhancement, and accessibility. My Books is the sole
browse surface for synced catalog metadata; there is no live OPDS browse or
search surface.

## Shipped cover-grid extension

[Book Covers](book-covers.md) defines the shipped presentation for the repeated
active-language results: one responsive cover grid with title and author always
visible, Reading Journey membership as its only relationship signal, and labeled
Journey plus secondary actions. It preserves this feature's search, deterministic
ordering, paging, empty states, language scope, and progressive-enhancement
contracts. Reading Journey is the sole learner surface for evidence and Primary
Goal presentation.

## Requirements

### Active-language scope

- My Books always presents the active study language's Books. Browse, paging,
  and search are scoped to it; the shell's active-language switcher changes the
  scope instead of a per-page control.
- Books without a chosen language belong to no language partition and appear
  only through an out-of-band **needs language** strip (display-only: fix the
  language in the catalog, then re-sync; no per-book actions), never as a
  filter in the active-language collection. The distinct out-of-band browse
  state is `/library?needs-language`.
- The active language is context, not a finding aid: it is carried by the page
  heading and the switcher, not repeated per row.

### Scoped collection search and paging

- One text search filters the active study language's My Books collection,
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
- Reading Journey membership is the only relationship signal. Evidence and
  Primary Goal state belong to Reading Journey and never turn the collection into
  a metric-led dashboard.
- Missing author or edition/year is stated or omitted without inventing
  metadata.
- Choosing an analyzed Journey member opens its canonical
  `/journey#journey-book-{bookID}` anchor. Metadata-only and otherwise
  incomplete Books remain operable from their rows through Journey membership
  and catalog actions; they have no detail page.
- The list does not expose batch-select-then-analyze behavior.

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
| Empty collection | Explain My Books and the Catalogs path to catalogue setup. | Add a catalogue connection |
| Collection available | Show scoped search, a cover-led collection for the active language, and paging. | View in Reading Journey or Add to Reading Journey |
| Cover unavailable or pending | Keep the title and author visible beside a truthful placeholder. | View in Reading Journey or Add to Reading Journey |
| Needs-language Books exist | Show the out-of-band **needs language** strip; do not infer a language. | Fix catalog metadata and re-sync |
| Search empty | Retain the query within the active language and state that the local collection has no match. | Revise or clear search |
| Later page becomes empty | Return to the nearest valid page without losing the query context. | Continue browsing |
| Enhancement failed | Keep or restore the ordinary server-rendered form/link path. | Submit normally |

## Non-goals

- Live OPDS search, catalog discovery, or upstream pagination.
- Cross-language browse or search; an "All languages" default.
- Batch selection or batch analysis.
- A default readiness ranking, recommendation, or “best next book.”
- Changing Book identity, membership, evidence, Journey, or Primary Goal
  contracts.
- Requiring JavaScript, drag interactions, or a spatial collection canvas.

## Acceptance criteria

- Browse, search, and paging return only Books in the active study language.
- A Book without a chosen language is reachable only through the **needs
  language** strip and becomes a language Book after fix + re-sync.
- Query and page combinations return stable owner-scoped results and preserve
  their state in navigable URLs.
- Search covers the active language's local collection and performs no OPDS
  request.
- Analyzed Journey-member items link to their canonical Reading Journey anchor;
  other rows do not link to a detail page.
- Empty, no-match, needs-language, and paging-boundary states are distinct.
- The full workflow is keyboard-operable and usable without JavaScript; HTMX
  enhancement preserves focus, announcements, and URL meaning.
- Narrow and long-content cases retain bibliographic identity and usable actions.
