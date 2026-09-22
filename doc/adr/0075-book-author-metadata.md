# ADR 0075: Catalogue-sourced Book author metadata

Status: **Accepted** · Date: 2026-09-22 · Author: Justin + opencode

## Context

Reading Journey rows are book-led surfaces and must identify a Book by title and
author when that metadata is available. The current Book model stores title but
not author, even though metadata-only Books can appear in Reading Journey before
an EPUB is acquired. EPUB metadata therefore cannot be the canonical source for
this identity.

## Decision

The connected learner-owned OPDS catalogue is the canonical source of mutable
Book author metadata, consistent with ADR 0041 and ADR 0047. Catalogue sync and
per-book refresh store the feed's author display value in `books.author` and
replace it on later reconciliation. The field is a non-null text value whose
empty string means that the catalogue supplied no author.

An entry's Atom `<author><name>` values are joined with `, ` in feed order. When
no Atom author is present, `dc:creator` is used. This is a presentation string,
not a contributor model; preserving structured or separately editable
contributors is out of scope.

Reading Journey renders the stored author subordinate to and immediately after
the title. It renders no placeholder or inferred author when the stored value is
empty. Acquiring an EPUB does not replace catalogue metadata, and source
material read models project the Book author rather than inventing an EPUB-only
fallback.

## Consequences

- Metadata-only and acquired Books have one owner-scoped, refreshable author
  identity on every book-led surface.
- Existing Books migrate with an empty author and remain valid without a data
  backfill.
- Missing or malformed upstream author metadata remains visibly absent instead
  of being guessed from content.
- A future structured contributor model would require a new decision and a
  migration; this text field deliberately supports the current design contract.

## Related

- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived](0047-acquisition-folded-into-analysis.md)
- [Catalogue sync feature](../features/catalog-sync.md)
