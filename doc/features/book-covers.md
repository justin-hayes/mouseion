# Book Covers

Status: **Implemented** · Date:
2026-09-23

## Motivation

My Books is a local bibliographic collection, but its repeated text rows make a
large collection slower to recognize visually than the covers on a physical
bookshelf. Catalog-supplied cover images can make browsing more immediate while
title and author preserve trustworthy Book identity. Reading can reuse
the same images at lower prominence without becoming a second cover browser.

This feature extends [My Books Collection Browsing](collection-browsing.md) and
[Catalog Sync](catalog-sync.md). Its persistence and source boundary are governed
by [ADR 0077](../adr/0077-catalog-supplied-book-covers.md).

## Goals

- Make the Book cover the leading visual element in the My Books collection
  when one is available.
- Replace the My Books result rows with one responsive cover grid rather than a
  learner-selectable grid/list preference.
- Keep title and author visible for every Book; a cover is never the sole
  learner-facing identifier.
- Communicate workflow disposition without duplicating Reading evidence or
  current-reading state.
- Add modest cover thumbnails to Reading while preserving semantic list order,
  evidence, recovery, and current-reading actions.
- Retain catalog-supplied covers locally so browsing does not depend on the
  catalog being online.

## Scope

The feature applies to active-language My Books results at `/library`, current
and the pinned Current reading and To Read Book rows, and catalog sync. Search, alphabetical
ordering, paging, language scope, and Book identity remain unchanged.

My Books shows the active language's chosen Books in two tabs, **To Read** and
**Read**. The retired needs-language strip has no replacement: a Book without a
chosen language is not listed in My Books.

## Book cover metadata

- A Book cover is catalog-supplied, mutable Book metadata with the Catalog entry
  that supplied it retained as provenance.
- Mouseion discovers covers only from image links advertised by a learner-owned
  OPDS Catalog entry. It does not infer an image from title, author, ISBN, or
  acquired EPUB content.
- When a Book has aliases from more than one Catalog connection, an existing
  selected cover source remains stable. A later sync of another alias does not
  replace it merely because that alias reconciled later.
- When no selected cover remains, the first alias whose advertised image is
  fetched and validated successfully may become the selected source. Other
  aliases remain fallbacks rather than competing display images. There is no
  Catalog priority in this feature: an atomic first-success selection resolves
  concurrent initial retrievals, and later completion cannot displace the winner.
- A successful refresh of the selected Catalog entry may replace its image. Each
  successful complete sync refetches every entry's advertised image, including
  one whose link is unchanged, so an image replaced at the same URL is picked
  up. The prior validated image remains visible until the replacement has been
  fetched and validated, then the replacement is swapped in atomically.
- If a successful complete reconciliation finds the selected Catalog entry but
  it contains no recognized cover-image link, that is explicit absence: Mouseion
  removes the selected image and another alias may supply a fallback. A missing
  entry, failed or incomplete feed walk, unreachable Catalog, or advertised but
  invalid image is not explicit absence and leaves the retained image unchanged.
- Deleting a Catalog connection or removing a Book from active My Books
  membership does not delete its retained cover, consistent with existing Book
  and artifact retention.

## Storage

Retained cover images are keyed by Catalog entry, not by Book (ADR 0130). This
section records the shape settled under ADR 0038 before any SQL.

- **Entry store.** `catalog_entry_covers` is keyed `(owner_id, connection_id,
  entry_identifier)`, the same identity as `catalogue_entries` and the
  catalog-entry alias (ADR 0044). One row holds the retrieval state
  (`pending`, `available`, `unavailable`; absence of a row means no cover is
  advertised), the `advertised_at` retrieval-generation fence, and for an
  available image its media type, dimensions, content hash, bytes, and fetch
  time, plus an operational failure reason.
- **No foreign keys.** The row does not reference `opds_connections`,
  `catalogue_entries`, or `books`, so an image a Book uses outlives its
  Catalog connection. Removal is application-driven: an entry that leaves the
  index loses its cover in the same transaction as index reconciliation unless a
  Book selects it, and
  deleting a connection removes the covers of its entries that no Book uses.
  Ownership is enforced by an `owner_id` predicate on every read.
- **Book reference.** `book_covers (owner_id, book_id)` is only the Book's
  selection pointer: `selected_connection_id` and `selected_source_identifier`
  name an entry row through a composite foreign key, and the table holds no
  image, state, or diagnostics. A Book without a selection has no row. The
  first alias whose image becomes available selects it (written in the same
  transaction as the image, and when a Book is added for an entry whose image is
  already retained), and a selection whose entry has no image yet gives way to
  an available one. A Book's presentation state is derived: the selected
  entry's state, or, while nothing is selected, `pending` while an alias entry
  is pending, `unavailable` when its alias entries all failed, and `none`
  otherwise (the `book_cover_states` view, which `my_books_evidence` joins).
- **Retrieval.** Every index entry that advertises a cover is retrieved
  independently of Books, by a job keyed by owner, connection, entry, and
  `advertised_at`. A completion whose `advertised_at` no longer matches the row
  is dropped. The previous image stays until a replacement validates. Explicit
  absence in a complete sync removes the row.
- **Lifecycle.** An entry cover a Book selects survives its entry leaving the
  index and its connection being deleted; covers no Book selects are removed
  with either. A retrieval still pending when its connection is deleted becomes
  `unavailable`. When a selected entry explicitly stops advertising a cover, in
  one transaction the Book's selection is demoted to another alias with an
  available image (or removed) and then the entry row is deleted.
- **Migration sequence.** Expand (`catalog_entry_covers`, structural only), a
  separate data-only backfill from `book_covers` (000065: idempotent, fetches
  nothing), then contract (000066: `book_covers` shrinks to the pointer,
  `book_cover_candidates` is dropped, the foreign key is added; it refuses to
  run if an available per-Book image has no selected entry, or a selection has
  no entry row), shipped in one
  deploy. Covers of Books still waiting at upgrade are retrieved at the next
  Catalog sync; queued retired per-Book retrieval jobs are discarded by a separate data-only migration (000067).
- **Catalog cards.** The OPDS Catalog card shows the retained image through an
  owner-scoped endpoint, or the Title plate while none is retained. The upstream
  image URL never reaches the browser.

## Retrieval and persistence

- Catalog reconciliation records advertised cover provenance and schedules
  cover retrieval independently. Catalog sync does not wait for every image and
  does not fail because an optional cover could not be retrieved.
- Retrieval completion is conditional on the current cover source: a stale
  success or failure cannot restore an explicitly absent source, replace a
  newer fallback, or change its presentation state or diagnostics.
- Retrieval uses the same owner-scoped connection credentials, origin
  restrictions, and redirect safety as other OPDS requests. Catalog URLs and
  credentials are never exposed to the browser.
- Initial retrieval accepts only content-sniffed JPEG and PNG raster images. It
  reads at most 8 MiB, rejects images above 40 megapixels, and rejects SVG,
  animated images, unsupported formats, malformed files, and media-type claims
  that disagree. When supplied, the OPDS declaration, response `Content-Type`,
  and decoded media type must agree after normalization; omitted optional claims
  remain acceptable for valid JPEG or PNG content.
- Mouseion preserves aspect ratio, does not upscale, and normalizes the selected
  cover to a JPEG or PNG no larger than 600 by 900 pixels. It retains that one
  display raster, not an unlimited original or append-only image history.
- Cover bytes live in PostgreSQL and are excluded from ordinary Book list and
  evidence projections. A dedicated authenticated, owner-scoped endpoint loads
  and serves bytes only for an active My Books member. (The Catalog card's
  entry-cover endpoint is separate: it serves a retained entry image to the
  owner of that Catalog connection.) Missing, unavailable,
  removed-from-view, and cross-owner references all return the same not-found
  response; conditional requests are evaluated only after authorization.
- The image response supplies intrinsic dimensions, a content-derived validator,
  and private browser caching. Cross-owner and missing references reveal no Book
  or Catalog metadata.
- Retrieval failures may be recorded in operational diagnostics and retried by
  a later catalog sync. They do not add a per-Book error banner, a new primary
  destination, or a learner-facing retry action.

## My Books cover grid

The result collection is a native unordered list laid out with CSS Grid. It is
not an application-style ARIA grid and does not introduce managed arrow-key
navigation. Search and paging continue to update the same result region with a
complete server-rendered fallback.

Each Book item presents, in order:

1. A consistent portrait cover frame, approximately 2:3. The complete image is
   preserved with `object-fit: contain`; Mouseion does not crop catalog artwork
   to fill the frame.
2. The full title in the bibliographic reading face and the author when supplied,
   always visible beneath the cover. Long identity text wraps rather than being
   truncated into ambiguity.
3. One status badge for the Book's visible bucket: **Currently reading**, **To
   Read**, or **Read**. Beneath it, a concise margin note states current
   evidence when there is any: the latest completion on a Read Book, preparation
   or acquisition in progress or failed (with its recovery action), or the
   current analysis and deck state. A Book with reading history also shows its
   completion count and latest completion. The item never ranks Books or
   recommends a next action.
4. One visible, labeled Reading action, **View in Reading**, on the current
   Book and on every To Read Book. A Read Book has no visible Reading action.
5. A labeled native **More actions** disclosure containing **Mark as previously
   read** for an unstarted To Read Book, **Read again** for a Book with reading
   history, and **Remove from My Books**, which is disabled with a reason for
   the current Book. Remove ends membership without
   changing the Book's evidence, history, or artifacts. There is no per-Book
   metadata refresh: a catalog-linked Book follows its entry's title, author,
   and cover at the next sync.

When a Reading anchor exists, the cover and title form one link to that
anchor. Otherwise neither performs an implicit action; the explicit Reading
control remains the only way to add membership. There is no Book detail page and
the cover never triggers a context-dependent mutation.

The grid uses a medium-density auto-fitting layout with items approximately
10-12rem wide where space permits and two columns at ordinary compact widths.
It reduces columns rather than introducing horizontal page scrolling. The
25-Book page size remains unchanged until measured image and layout performance
provides evidence for another value.

## Reading thumbnails

The current Book and each To Read candidate reserve one modest,
consistently sized thumbnail column before the existing Book identity. A real
cover uses the same complete-image fitting rule as My Books; a missing or
unavailable image uses a subdued placeholder so titles and controls remain
aligned across the ordered list.

The thumbnail is supporting identity. Reading retains its semantic list, title,
author, current-reading state, evidence, recovery,
preparation, and consequential-action behavior. This feature does not simplify
or reorder those contracts.

## Cover presentation states

| State | My Books presentation | Reading presentation |
|---|---|---|
| Valid retained cover | Complete image in the fixed-ratio frame | Complete image in the thumbnail frame |
| No cover advertised | Title plate with **No cover available** | Subdued aligned Title plate |
| Retrieval pending | Title plate with **Cover pending** | Subdued aligned Title plate |
| Advertised cover unavailable with no prior valid image | Title plate with **Cover unavailable** | Subdued aligned Title plate |
| Replacement pending | Existing validated image | Existing validated image |
| Replacement invalid or retrieval failed | Existing validated image | Existing validated image |

The Title plate (ADR 0130) composes the text's own title, Greek title when it has
one, and author on a tinted surface. It replaces the former title initial in the
Catalog, My Books and Reading, and is the image of every Classical Reading unit,
which has no catalog cover. It is presentation, not metadata: never stored or
called a cover. The Catalog card shows the retained entry cover, or the Title plate while none
is retained.

These are quiet visual metadata states, not Book status badges. Cover failures do
not displace title, author, disposition, or the learner's current task.

## Accessibility and progressive enhancement

- Real cover images use empty alternative text because the adjacent visible
  title supplies Book identity. Title plates are hidden from the accessibility
  tree; their state is not required to identify or operate the Book.
- A linked cover and title create one focus stop whose accessible name comes
  from the visible title. Do not create duplicate image and title links.
- Reading and disclosure controls retain visible text in every layout, and every
  action has an accessible name. Controls are never available only on hover,
  focus, or pointer gesture.
- Native list, link, button, `details`, `summary`, and form behavior remains
  usable without JavaScript. Keyboard order follows document order from Book
  identity to Reading action to secondary actions.
- Focus indicators, control targets, contrast, and live-region behavior follow
  the existing WCAG 2.2 AA design-system contract.
- Images reserve intrinsic space and load lazily when below the viewport. The
  fixed frame prevents layout shift; reduced motion is respected and no shimmer
  or animated loading treatment is introduced.

## Non-goals

- External image or bibliographic providers.
- Extracting covers from acquired EPUBs.
- Learner cover upload, editing, cropping, or source selection.
- A saved or temporary list/grid preference.
- Generated imitation covers presented as a cover. The Title plate is the one
  composed image; it is labelled as no cover and never retained.
- A new Book detail page or making the cover an implicit mutation control.
- Changing search, alphabetical ordering, active-language scope, or paging.
- Moving Reading evidence, recovery, or current-reading controls
  into My Books.
- Making Catalog sync wait for optional image retrieval or treating a cover
  failure as a sync failure.

## Acceptance criteria

- Catalog image links are discovered without exposing owner credentials or
  allowing cross-origin/redirect escape, and validated images remain available
  when the Catalog is offline or its connection is deleted.
- Multiple Catalog aliases do not make a displayed cover change according to
  ordinary sync order; concurrent initial success, replacement, explicit
  absence, invalid-image, and failed-refresh cases follow the selected source
  contract.
- Retrieval accepts only decoded JPEG/PNG input within the 8 MiB and 40-megapixel
  limits and stores one aspect-preserving display raster no larger than 600 by
  900 pixels; rejected input leaves Book and sync state usable.
- My Books renders an active-language, searchable, paged native list as a
  responsive cover grid while retaining complete title and author identity.
- My Books communicates workflow disposition, not analysis evidence or current
  reading, and offers labeled Reading, refresh, and removal paths.
- Reading shows aligned modest thumbnails without changing its semantic list,
  evidence, recovery, or current-reading behavior.
- Missing, pending, unavailable, and replacement cover states remain stable,
  truthful, and usable without JavaScript.
- Cover and placeholder semantics do not duplicate Book names for assistive
  technology; all actions remain keyboard- and touch-operable.
- Owner-scoped image requests cannot retrieve another learner's bytes, ordinary
  collection queries do not load image bytes, and bounded lazy images do not
  introduce page-level horizontal overflow or avoidable layout shift.
