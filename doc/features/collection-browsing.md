# My Books Collection Browsing

Status: Proposed · Date: 2026-09-02

## Motivation

My Books is the learner's broad bibliographic collection, not a queue or a
readiness ranking. As metadata-first catalogue sync increases its size, a single
unpaged list is no longer sufficient for finding a title, author, edition, or
language. The canonical information architecture already promises a searchable
bibliographic catalogue; this feature makes that promise concrete.

## Goal

Let learners find and open a Book in a large local My Books collection through
language grouping, paging, and global text search while keeping bibliographic
identity and learner intention ahead of analysis evidence.

## Scope

This feature applies to My Books at `/library`. It defines collection controls,
row hierarchy, progressive enhancement, and accessibility. It does not change
catalogue browsing or OPDS search.

## Requirements

### Language grouping

- My Books presents one pill for each language represented in the local active
  collection, plus an **Unknown language** pill for Books whose language state is
  unknown or absent.
- Pill counts and result counts derive from the same owner-scoped collection
  query and remain consistent with active My Books membership.
- Selecting a language preserves the current text query where possible and
  returns a paged subset for that language.
- Language is a finding aid, not a readiness rank. Readiness does not determine
  the default language or order.

### Global collection search and paging

- One text search filters the learner's complete local My Books collection,
  including metadata-only and acquired Books. It is explicitly not a live OPDS
  query and does not depend on an upstream connection being available.
- Search combines predictably with the selected language and paging controls.
- Paging uses stable, deterministic ordering and preserves the current language
  and query in real links and forms.
- Empty-collection, search-empty, and language-empty states remain distinct and
  offer an appropriate way to clear a filter, revise a query, or use Add books.

### Row hierarchy and book selection

- Rows lead with title and author, followed by edition or publication year and
  language when available.
- Primary Goal / Reading Journey relationship and concise trustworthy evidence
  state follow bibliographic identity. Evidence never displaces the title or
  turns the collection into a metric-led dashboard.
- Missing author, edition/year, or language is stated or omitted without
  inventing metadata.
- Choosing one Book opens `/books/{id}`. This is the manual selection for scope
  review, lazy content acquisition, or analysis decisions.
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
- Narrow layouts preserve title, author, edition/year, language, and the book
  link without page-level horizontal scrolling. Long titles and names wrap
  rather than being truncated into ambiguity.

## States

| State | Required presentation | Primary action |
|---|---|---|
| Empty collection | Explain My Books and the existing Add books path. | Add books |
| Collection available | Show global search, language pills with counts, deterministic rows, and paging. | Open a book |
| Unknown-language results | Group Books with no chosen language without inferring one. | Open a book or update it through an owned workflow |
| Search empty | Retain the query and selected language and state that the local collection has no match. | Revise or clear search |
| Language empty after search | Retain both controls and explain the combined filter. | Clear one filter |
| Later page becomes empty | Return to the nearest valid page without losing the query/language context. | Continue browsing |
| Enhancement failed | Keep or restore the ordinary server-rendered form/link path. | Submit normally |

## Non-goals

- Live OPDS search, catalogue discovery, or upstream pagination.
- Batch selection or batch analysis.
- A default readiness ranking, recommendation, or “best next book.”
- Changing Book identity, membership, evidence, Journey, or Primary Goal
  contracts.
- Requiring JavaScript, drag interactions, or a spatial collection canvas.

## Acceptance criteria

- Language pills include an unknown/no-language bucket and accurate counts.
- Query, language, and page combinations return stable owner-scoped results and
  preserve their state in navigable URLs.
- Search covers the complete local collection and performs no OPDS request.
- Rows follow the information-architecture hierarchy and link to `/books/{id}`.
- Empty, no-match, unknown-language, and paging-boundary states are distinct.
- The full workflow is keyboard-operable and usable without JavaScript; HTMX
  enhancement preserves focus, announcements, and URL meaning.
- Narrow and long-content cases retain bibliographic identity and usable actions.
