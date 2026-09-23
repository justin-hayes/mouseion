# ADR 0077: Retain catalog-supplied Book covers

Status: **Accepted** · Date: 2026-09-23 · Author: Justin + opencode

Extends the optional Book metadata boundary in
[ADR 0035](0035-my-books-membership-and-source-provenance.md) and the
metadata-first reconciliation contract in
[ADR 0041](0041-catalog-sync-metadata-first.md). It amends the My Books evidence
placement retained by [ADR 0050](0050-active-study-language.md) and
[ADR 0057](0057-retire-language-view-panel.md): Reading Journey becomes the sole
learner-facing Book evidence surface when the cover-grid target ships.

## Context

My Books will become a cover-led grid, and Reading Journey will show modest Book
thumbnails. Metadata-only Books need covers before EPUB acquisition, catalogs may
require owner credentials to serve images, and the browser must not depend on a
live upstream connection. A Book may also have Catalog-entry aliases from more
than one connection, while the current Book model designates no authoritative
alias whose sync order should control artwork.

Mouseion's only mutable durable store is PostgreSQL. EPUB snapshots and prepared
deck artifacts already use `bytea`; the deployed web process has no durable
writable filesystem or object-storage dependency.

## Decision

A Book cover is optional, mutable, catalog-supplied Book metadata with its exact
owner-scoped Catalog-entry provenance retained. Initial support reads only image
links advertised by connected OPDS catalogs. Mouseion does not consult external
bibliographic services, extract EPUB covers, infer images, or accept learner
uploads.

The first Catalog alias that successfully establishes a selected cover remains
the source until it explicitly advertises no cover. There is no Catalog priority:
an atomic first-success selection resolves concurrent initial retrievals, after
which syncing another alias cannot replace the winner merely because it ran
later. A refresh from the selected source keeps the old validated image while a
changed image is fetched, then replaces it atomically. A successful complete
reconciliation that finds the entry with no recognized image link removes the
cover; a missing entry, incomplete or failed reconciliation, or invalid image
preserves it. When no selected cover remains, another alias may establish the
fallback.

Catalog reconciliation records cover provenance and schedules bounded retrieval
without waiting for image completion. Cover failure neither fails catalog sync
nor changes Book, content, analysis, Journey, or Goal state. Retrieval uses the
existing owner credentials and origin/redirect confinement, validates and
normalizes one display-ready raster, and records diagnostics for later retry.
Initial support accepts only content-sniffed JPEG and PNG input, reads at most
8 MiB, rejects images above 40 megapixels, and stores one aspect-preserving JPEG
or PNG no larger than 600 by 900 pixels. SVG, animation, malformed content,
unsupported formats, and advertised/decoded media mismatches are rejected.

The normalized bytes, media metadata, content validator, selected Catalog-entry
provenance, and retrieval state are stored in a dedicated PostgreSQL relation.
Cover bytes are not added to `books` or ordinary Book/read-model projections.
One authenticated owner-scoped endpoint reads and serves the selected image with
private caching; the browser never receives the upstream URL or credentials.

Deleting a Catalog connection or removing active My Books membership preserves
the retained cover with the Book and its other artifacts. Cover versions are not
historical artifacts: successful replacement updates the selected display image
in place rather than retaining an append-only image history.

My Books becomes a cover-led grid whose only learner-state signal is **In Reading
Journey**; analysis evidence and Primary Goal state remain in Reading Journey.
Title and author remain visible beneath every cover, and explicit labeled actions
own Journey membership, metadata refresh, and removal. Reading Journey adds the
same image as a modest aligned thumbnail without changing its ordered-list,
evidence, Goal, forecast, or recovery contracts.

## Consequences

- Metadata-only and acquired Books can use the same cover before and after EPUB
  acquisition, including while the catalog is unavailable.
- Catalog credentials, upstream URLs, cross-origin redirects, malformed images,
  and unbounded payloads remain behind the server's owner-scoped trust boundary.
- Database backups include covers without adding a filesystem volume or object
  store, at the cost of bounded additional PostgreSQL storage.
- Ordinary collection reads remain small because image bytes require a separate
  request; private content validators allow browser caching after authorization.
- Stable source selection avoids artwork changing with sync scheduling, but a
  learner cannot manually prefer another alias in this feature.
- Catalog sync remains metadata-first: it never downloads EPUB content or starts
  analysis, while optional cover retrieval is independent asynchronous metadata
  work.
- My Books becomes simpler and more visual, while Reading Journey is the sole
  place that combines Book identity with analysis and Goal evidence.

## Alternatives considered

- **Serve upstream image URLs directly.** Rejected because authenticated catalogs
  could expose credentials or fail in the browser, cross-origin behavior would
  escape Mouseion's trust boundary, and covers would disappear while offline.
- **Proxy every request without retaining bytes.** Rejected because browsing
  would remain coupled to upstream availability and repeatedly load the catalog.
- **Store images on the filesystem or in object storage.** Rejected because both
  introduce deployment and backup infrastructure absent from Mouseion today.
- **Extract EPUB covers.** Rejected because metadata-only Books need covers before
  acquisition and acquired content is not the canonical bibliographic source.
- **Use last-reconciled alias wins.** Rejected because connection schedules would
  make a Book's artwork change without learner intent or a stable source rule.

## Related

- [Book Covers](../features/book-covers.md)
- [ADR 0035: My Books membership and source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0044: Catalogue-entry identity is connection-scoped](0044-catalogue-entry-connection-scoped-identity.md)
- [ADR 0050: Active study language scopes learner-facing work](0050-active-study-language.md)
- [ADR 0057: Retire the language-view panel](0057-retire-language-view-panel.md)
