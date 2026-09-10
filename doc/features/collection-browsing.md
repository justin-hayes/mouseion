# My Books Collection Browsing

Status: Proposed · Date: 2026-09-02 · Updated: 2026-09-07

## Motivation

My Books is the learner's broad bibliographic collection, not a queue or a
readiness ranking. As metadata-first catalogue sync increases its size, a single
unpaged list is no longer sufficient for finding a title, author, edition, or
language. The canonical information architecture already promises a searchable
bibliographic catalogue; this feature makes that promise concrete.

## Goal

Let learners find and open a Book in a large local My Books collection through
paging and text search scoped to the active study language, while keeping
bibliographic identity and learner intention ahead of analysis evidence. My
Books is always browsed within the active study language; there is no
cross-language "All languages" default ([ADR 0050](../adr/0050-active-study-language.md)).

## Scope

This feature applies to My Books at `/library`. It defines collection controls,
row hierarchy, progressive enhancement, and accessibility. My Books is the sole
browse surface for synced catalogue metadata; there is no live OPDS browse or
search surface.

## Requirements

### Active-language scope

- My Books always presents the active study language's Books. Browse, paging,
  and search are scoped to it; the shell's active-language switcher changes the
  scope instead of a per-page control.
- Books without a chosen language belong to no language partition and appear
  only through an out-of-band **needs language** strip (display-only: fix the
  language in the catalogue, then re-sync; no per-book actions), never as a
  filter or browse state in the active-language collection; the distinct
  out-of-band state is `/library?needs-language`.
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
  catalogue setup through `/connections` and the Add books workflow.

### Row hierarchy and book selection

- Rows lead with title and author, followed by edition or publication year when
  available; the language tag is carried by the active-language heading, not
  repeated per row.
- Primary Goal / Reading Journey relationship and concise trustworthy evidence
  state follow bibliographic identity. Evidence never displaces the title or
  turns the collection into a metric-led dashboard.
- Missing author or edition/year is stated or omitted without inventing
  metadata.
- Choosing an analyzed Journey member opens `/journey/{bookID}`. Metadata-only
  and otherwise incomplete Books remain operable from their rows through
  Journey membership and catalogue actions; they have no detail page.
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
| Empty collection | Explain My Books and the Add books path to catalogue setup. | Add a catalogue connection |
| Collection available | Show scoped search, deterministic rows for the active language, and paging. | Open a book |
| Needs-language Books exist | Show the out-of-band **needs language** strip; do not infer a language. | Fix catalogue metadata and re-sync |
| Search empty | Retain the query within the active language and state that the local collection has no match. | Revise or clear search |
| Later page becomes empty | Return to the nearest valid page without losing the query context. | Continue browsing |
| Enhancement failed | Keep or restore the ordinary server-rendered form/link path. | Submit normally |

## Non-goals

- Live OPDS search, catalogue discovery, or upstream pagination.
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
- Analyzed Journey-member rows link to `/journey/{bookID}`; other rows do not
  link to a detail page.
- Empty, no-match, needs-language, and paging-boundary states are distinct.
- The full workflow is keyboard-operable and usable without JavaScript; HTMX
  enhancement preserves focus, announcements, and URL meaning.
- Narrow and long-content cases retain bibliographic identity and usable actions.
