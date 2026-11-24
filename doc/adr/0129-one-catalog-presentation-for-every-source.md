# ADR 0129: One Catalog presentation for every source

Status: **Accepted — implementation pending** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in a grilling session on 2026-11-24. Amends
[ADR 0116](0116-classical-catalog.md) and
[ADR 0117](0117-one-catalog-my-books-holds-chosen-books.md). Builds on
[ADR 0128](0128-a-classical-book-is-a-whole-work.md) and is implemented after
it. Covers come from
[ADR 0130](0130-retained-catalog-entry-covers.md); the shell's study-language
picker is settled by
[ADR 0131](0131-the-url-carries-the-study-language.md).

## Context

ADR 0117 made the Catalog the one surface for every source, but the Classical
catalog kept the presentation ADR 0116 gave it: its own "Classical catalog"
button and page at `/catalogs/classical`, an author-grouped index with each
Work's Opening, a length strip on one page-wide word scale, edition and passage
facts in a margin, and "Add and prepare". Each OPDS connection shows a paged
grid of cover cards with "Add" and "Adding downloads and analyzes the text." The
learner sees two unrelated screens where the model has one Catalog with sources
side by side.

The Catalog also carries two language controls: the shell's Study language
picker, which stores the Active study language and redirects to My Books, and
the Catalog's own Language filter, which is the one that filters. ADR 0116 says
the Classical catalog "is not scoped to the active study language", yet the
code shows it only when the filter is all languages or `grc`.

## Decision

**The Classical catalog is an ordinary source section of the Catalog.** Choosing
Ancient Greek in the Catalog's Language filter shows a section headed "Classical
catalog" built exactly like an OPDS connection's section, and choosing all
languages shows it beside them. Sources stay side by side and are never merged.
The separate Classical index page and its button are retired;
`/catalogs/classical` and its retired author URLs answer with a permanent
redirect to `/catalogs?language=grc`. The Work page remains, reached from the
card's title.

**One card for every source.** Each card is one thing the learner can add: an
OPDS Catalog entry, or a Classical Reading unit (a Work in one Edition, so a
second Edition is a second card told apart by its Edition label). A card shows
its image, title, author, one source-specific detail line, its ownership state,
and **Add** and **I've read this**. A Classical card's title carries the Greek
title beneath it and its detail line is the Edition label. Its image is a
**Title plate**, as is any OPDS card without a retained cover. The Opening,
length strip, passage range, translation availability and the contents list
move to the Work page. OPDS entries have no detail page; the index stores
nothing beyond the card.

**Every section is ordered and paged alike:** by title then author, paged per
section with one shared page size of 12. The Classical author grouping is
retired.

**One Add wording.** Every source's Add says **Add** with one line of copy that
is true of both kinds of Book preparation. The Classical licence credit moves to
a footer on the Classical section.

**The Catalog's Language filter is its only language control.** The Catalog is
not a per-language surface: the shell's Study language picker is not shown on
it. The filter defaults to the last study language on arrival, is never stored,
and always offers Ancient Greek because the installation always carries the
Classical catalog, so a learner can start a language they do not study yet.
The Classical section follows the filter like any source section; ADR 0116's
"not scoped to the active study language" now means it is never hidden by the
learner's study languages, only by the filter.

Adding a Book never changes the learner's study-language context.

## Considered options

- **Merge every source into one result list.** Rejected: it reverses ADR 0117's
  "never merged", and a learner needs to know which source will supply the text.
- **Keep the Classical presentation and only add images.** Rejected: the
  difference is the layout, not the missing image. The Opening and length strip
  are useful, but bulky, and they belong on the Work page.
- **Keep the shell picker on the Catalog and have it drive the filter.**
  Rejected: the shell picker offers only study languages, and the Catalog must
  offer languages the learner does not study yet.

## Consequences

- ADR 0116's catalog page set (the 2026-11-19 amendment's author-grouped index
  with Openings and length strips) and its "built-in, read-only record above the
  learner's OPDS connections" are superseded for the Catalog's results. The Work
  page and every Reading-unit rule are unchanged.
- ADR 0117's per-source presentation ("each OPDS source shows its index entries
  as a cover grid", the Classical index "keeps its author grouping") becomes one
  shared card grid.
- The design documents' Catalog entries (`screen-inventory.md`,
  `components.md`, `information-architecture.md`, `terminology.md`) are updated
  when this ships.
