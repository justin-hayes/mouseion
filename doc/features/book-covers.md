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
and To Read Book rows at `/reading`, catalog sync, and per-Book metadata refresh.
Search, alphabetical ordering, paging, language scope, and Book identity remain
unchanged.

The `/library?needs-language` diagnostic state remains a compact text-led list.
Its task is to identify metadata that must be corrected in the catalog, not to
browse or mutate the active-language collection.

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
- A successful refresh of the selected Catalog entry may replace its image. The
  prior validated image remains visible until the replacement has been fetched
  and validated, then the replacement is swapped in atomically.
- If a successful complete reconciliation finds the selected Catalog entry but
  it contains no recognized cover-image link, that is explicit absence: Mouseion
  removes the selected image and another alias may supply a fallback. A missing
  entry, failed or incomplete feed walk, unreachable Catalog, or advertised but
  invalid image is not explicit absence and leaves the retained image unchanged.
- Deleting a Catalog connection or removing a Book from active My Books
  membership does not delete its retained cover, consistent with existing Book
  and artifact retention.

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
  and serves bytes only for an active My Books member. Missing, unavailable,
  removed-from-view, and cross-owner references all return the same not-found
  response; conditional requests are evaluated only after authorization.
- The image response supplies intrinsic dimensions, a content-derived validator,
  and private browser caching. Cross-owner and missing references reveal no Book
  or Catalog metadata.
- Retrieval failures may be recorded in operational diagnostics and retried by
  a later catalog sync or explicit metadata refresh. They do not add a per-Book
  error banner, a new primary destination, or a learner-facing retry action.

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
3. A restrained disposition marker for Inbox, To Read, or Set Aside. My Books
   does not show analysis, acquisition, evidence, current reading, or next-action
   status on the grid item.
4. One visible, labeled Reading action: **Move to To Read** or **View in
   Reading**, accompanied by an icon.
5. A labeled native **More actions** disclosure containing textual **Refresh
   metadata** and **Remove from My Books** actions when each is eligible. Removal
   retains its existing consequential confirmation.

Every catalog-backed Book whose source connection remains available offers
**Refresh metadata**, whether or not EPUB content has been acquired. Refresh can
update title, author, or cover metadata and has no analysis or acquired-content
side effect.

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
| No cover advertised | Placeholder with **No cover available** | Subdued aligned placeholder |
| Retrieval pending | Placeholder with **Cover pending** | Subdued aligned placeholder |
| Advertised cover unavailable with no prior valid image | Placeholder with **Cover unavailable** | Subdued aligned placeholder |
| Replacement pending | Existing validated image | Existing validated image |
| Replacement invalid or retrieval failed | Existing validated image | Existing validated image |

These are quiet visual metadata states, not Book status badges. Cover failures do
not displace title, author, disposition, or the learner's current task.

## Accessibility and progressive enhancement

- Real cover images use empty alternative text because the adjacent visible
  title supplies Book identity. Placeholders are hidden from the accessibility
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
- Generated imitation covers based on title, author, initials, or color.
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
