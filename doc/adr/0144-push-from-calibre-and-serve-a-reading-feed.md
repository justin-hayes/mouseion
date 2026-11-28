# ADR 0144: Push from a Calibre library and serve a Reading feed

Status: **Proposed; needs human review** · Date: 2026-11-28 ·
Author: Justin + OpenCode

Tracked by [#2932](https://github.com/justin-hayes/mouseion/issues/2932)
(Track F: Calibre push and the Reading feed). Amends
[ADR 0041](0041-catalog-sync-metadata-first.md),
[ADR 0109](0109-retained-epub-from-catalog-acquisition.md) and
[ADR 0117](0117-one-catalog-my-books-holds-chosen-books.md), and the ADR 0142
proposed in [#2865](https://github.com/justin-hayes/mouseion/issues/2865). It
relies on the Device credential of the ADR 0143 proposed in
[#2868](https://github.com/justin-hayes/mouseion/issues/2868).

## Context

EPUB Books reach Mouseion only from a learner-owned OPDS catalog connection,
in practice Calibre-Web serving a Calibre library. Mouseion syncs a Catalog
index from it (ADR 0041, ADR 0117), the learner adds an entry, and acquisition
fetches the Catalog file and keeps only the Retained EPUB, without images,
fonts, audio or video (ADR 0109).

The learner manages the library in Calibre on a laptop and would rather not run
Calibre-Web or manage a library in Mouseion. Their e-reader, a Boox Palma 2
running KOReader, never connects to that laptop.

The KOReader track (#2872) links device reports to Books by Document digest,
KOReader's partial MD5 of the file on the device. A digest links by itself only
when the device holds exactly the bytes Mouseion recorded. When the device gets
its copy from somewhere else, the match is luck. Calibre-Web can embed metadata
into each download, and Calibre writes metadata into the copies it sends to a
device. ADR 0142 records the Catalog file's digest for the copies already on
the device. It rejects both keeping the Catalog file and serving the Retained
EPUB, and it leaves a pass-through OPDS proxy as a later option.

## Decision

### Pushing from a Calibre library

1. A **Calibre library** is a new learner-owned concept: the learner's Calibre
   library, named in Mouseion, with its own revocable credential. It is built
   like a Device credential, but is not a Device. A Calibre library credential
   only pushes; a Device credential never pushes, and a session opens neither.
2. A Calibre plugin, kept in this repository and downloaded from Mouseion,
   **pushes** the selected books. A push is the learner's Add. It creates the
   Book, or restores the one the learner had, as To Read and starts EPUB
   preparation. **Send as read** creates or restores it as Read with a
   Previously read record, and prepares it too. A Book already in Reading keeps
   its place, and only its metadata and file change.
3. A pushed book is identified by the learner plus Calibre's book UUID,
   whichever Calibre library sends it. A Book added from a Catalog entry whose
   identifier is `urn:uuid:<that UUID>` is the same Book. Calibre-Web uses that
   form, so pushing a book that came from Calibre-Web creates no duplicate.
4. The push carries the EPUB, Calibre's title, its authors as Calibre displays
   them, its single language and its cover. Mouseion refuses a push, creating
   no Book, when the language is missing, several, English or not ready in the
   NLP service, when there is no EPUB, or when the file is over 512 MiB or not
   an EPUB. The plugin never converts formats.
5. A push is accepted once the file is kept and the Book exists. Preparation
   then runs in the background, its failures are Book preparation failures in
   Reading, and Retry works from the kept file. A push is idempotent on the
   Calibre UUID and the file's SHA-256: unchanged bytes update metadata only,
   and new bytes are a new content revision.
6. Once a Book has been pushed, pushes alone change its title, author and
   cover; catalog sync no longer does. Deleting a book in Calibre, or revoking
   or deleting a Calibre library, changes nothing in Mouseion.

### Keeping and serving the Library file

7. Mouseion keeps the **Library file**, the EPUB exactly as pushed, for each
   Book's current content revision, on a file volume addressed by its hash and
   not in PostgreSQL. It derives the Retained EPUB from it as ADR 0109 does,
   and analysis keeps reading only the Retained EPUB. A superseded Library file
   is deleted, but its Document digest is kept, so a copy already on a device
   still links. Backups must cover the volume as well as the database.
8. Mouseion records the Library file's Document digest when the push arrives,
   and a digest equal to it links automatically, as a Catalog file's does under
   ADR 0142.
9. The **Reading feed** is an OPDS 1.2 catalog served to the learner's
   Devices, signed in with the Device credential through KOReader's OPDS
   username and password. Its root has two acquisition feeds, To Read (the
   Current reading first) and Read, each across every study language. It lists
   only Books in Reading that have a Library file, and serves that file
   unchanged. It has no search and no Classical Books. A Book removed from
   Reading leaves the feed; the copy on the device stays.

### Catalog connections

10. OPDS catalog connections, catalog sync and the Catalog index stay until a
    later spec retires them, written once pushing has replaced Calibre-Web in
    practice. Its decisions, including what happens to Manage sources, existing
    indexes and catalog-entry aliases, are not made here. The Classical catalog
    is unaffected.

## Consequences

- ADR 0117's rule that a Book comes only from a Catalog entry, a Reading unit
  or Previously read gains a fourth way: a push. The Catalog stays the
  surface for finding texts from catalogs; pushed books are found in Calibre.
- ADR 0041's metadata-first rule does not apply to pushes, because content
  arrives with the learner's choice.
- ADR 0109 still governs what analysis reads. For pushed Books Mouseion now
  also keeps the full file, at the storage cost ADR 0109 avoided, but on a
  volume, where it neither grows the database nor passes through memory.
- ADR 0142 is extended rather than replaced. Its backfill still matters for the
  Calibre-Web copies already on the device. Books that came from Calibre-Web
  reach the Reading feed by being pushed once.
- A Book on the device that was downloaded from the Reading feed always links
  automatically, unless the digest is ambiguous for some other reason.
- Mouseion gains a second embedded plugin, built, tested and distributed like
  the KOReader plugin (#2882).

## Alternatives considered

- **Mouseion as a pass-through OPDS proxy of Calibre-Web.** Matches would be
  certain, but Calibre-Web would have to run all the time and be reachable, and
  Mouseion would still be a catalog client.
- **Push into a Catalog index, then Add in Mouseion.** Keeps ADR 0117 intact,
  but adds a second browsing step for books the learner already chose in
  Calibre.
- **A standalone utility over `calibredb`.** Works without the GUI, but cannot
  act on the books selected in Calibre, and duplicates what Calibre's plugin API
  provides.
- **Treat the plugin as a Device.** Reuses a concept, but a Device is a place
  where reading happens, and one credential would then open device protocols
  and pushes alike.
- **Keep the Library file in PostgreSQL.** Brings back the database growth and
  in-memory handling ADR 0109 removed.
- **Serve the Retained EPUB.** Rejected by ADR 0142: the learner would read
  without images, fonts or cover.
- **Sync metadata from Calibre automatically on every edit.** Hooks on edits
  are fragile, and a metadata edit that embeds into the file would create
  content revisions and reanalysis; explicit re-pushes do neither.
- **Identify a pushed book by Calibre library plus UUID.** A copied or
  re-registered library would duplicate every Book.
