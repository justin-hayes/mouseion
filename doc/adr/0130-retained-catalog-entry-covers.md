# ADR 0130: Retain covers for Catalog entries, and compose Title plates

Status: **Accepted; implemented** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in a grilling session on 2026-11-24. Amends
[ADR 0077](0077-catalog-supplied-book-covers.md). Supports
[ADR 0129](0129-one-catalog-presentation-for-every-source.md).

## Context

ADR 0077 retains a cover only for a Book. Catalog sync already records each
index entry's advertised cover link, but the Catalog never shows it: upstream
URLs must not reach the browser, so every OPDS card shows the title's initial.
The Classical catalog has no image source at all; its texts come from pinned
annotation and translation files that carry no artwork.

## Decision

**A retained cover belongs to a Catalog entry.** Catalog sync schedules the
retrieval of every index entry's advertised cover as independent, bounded
background work, under ADR 0077's existing rules: owner credentials, origin and
redirect confinement, content-sniffed JPEG or PNG, at most 8 MiB read and 40
megapixels decoded, one normalized image no larger than 600 by 900 pixels, and a
failure that never fails sync. Browsing the Catalog still never contacts the
OPDS server: a card shows its retained image once one exists, served from
Mouseion's own owner-scoped endpoint, and a Title plate until then.

**A Book's cover is resolved through its Catalog-entry alias.** There is one
image store, keyed by Catalog entry; the per-Book store is migrated onto it, so
adding a Book reuses the image the Catalog already showed. ADR 0077's selection
rule is unchanged: the first alias that establishes a cover stays the Book's
source until that entry advertises no cover.

**A retained entry cover lives as long as it is used.** It is removed when its
entry leaves the index, unless a Book's selected cover is that image; then it
stays with the Book, as ADR 0077 requires when a connection is deleted or
membership ends.

**Where no cover exists, Mouseion composes a Title plate** from the text's own
title (with the Greek title for a Classical Reading unit) and author. It is
presentation, never stored and never called a cover. It replaces the title
initial everywhere a cover may appear: the Catalog, My Books and Reading. Every
Classical Reading unit and every Classical Book shows a Title plate.

## Considered options

- **Proxy upstream images on demand without storing them.** Rejected for the
  reasons ADR 0077 gave: browsing would depend on upstream availability, and the
  Catalog's promise that browsing never contacts the OPDS server would break.
- **A second image table for index entries beside the per-Book one.** Rejected:
  an added Book would store the same image twice and fetch it again.
- **Curated public-domain images for Classical Works** (a bust, a manuscript
  leaf). Rejected for now: each needs sourcing and a recorded licence, and the
  image says nothing the title does not.

## Consequences

- Storage grows with the size of every synced index, not with My Books. The
  normalization bound keeps each image small; a later cap per connection can be
  added if a large catalog needs it.
- Changing the image store is a schema change. Its shape (keys, the reference
  from a Book's selected cover, and the migration of existing `book_covers`
  rows) is settled in the implementing spec under ADR 0038 before any SQL.
- The design documents' title-initial placeholder, shown "until locally
  retained index covers exist", is replaced by the Title plate.
