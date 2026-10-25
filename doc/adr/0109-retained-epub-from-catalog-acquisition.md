# ADR 0109: Retain a text-only EPUB from catalog acquisition

Status: **Accepted — implementation pending** · Date: 2026-10-25 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Specified in
[#2278](https://github.com/justin-hayes/mouseion/issues/2278). Amends the
acquisition steps of [ADR 0035](0035-my-books-membership-and-source-provenance.md)
and the "original bytes" wording of
[ADR 0103](0103-source-structure-and-poetry-evidence.md). Leaves ADR 0035's
digest-version-1 invariant unchanged.

## Context

Acquisition reads the catalog's EPUB into memory, refuses it over 100 MiB,
extracts from it, and stores the exact bytes as the content revision. ADR 0035
requires the digest to be "of the actual container bytes", and ADR 0103 says a
content revision "owns original bytes/digest".

Illustrated public-domain editions break this. Several French classics fail at
the 100 MiB limit (#2276), and eight acquired Books are already 70–99 MB, while
the median stored EPUB is 1.5 MB. The bulk is images and embedded fonts, which
Mouseion never reads. Extraction uses only the package document, navigation and
XHTML, and Book covers come from the catalog's cover link. Today those bytes cost
memory during acquisition, reject Books whose text is a few MiB, and grow the
database indefinitely.

Streaming the download alone is not enough. Extraction and `bytea` storage
still need the whole package in memory. The bytes Mouseion never uses have to
go before extraction and storage.

## Decision

### Terms

- **Catalog file**: the EPUB exactly as the catalog serves it.
- **Retained EPUB**: what Mouseion stores for a content revision. It is the
  Catalog file minus resources Mouseion never reads.

### Acquisition

1. Stream the Catalog file to a temporary file, computing its SHA-256 as it
   arrives. Cap it at 512 MiB, counted while streaming. Remove the temporary
   file on every outcome.
2. Derive the Retained EPUB from it with the slimming rules below.
3. Require the Retained EPUB to be at most 100 MiB, the existing in-memory
   bound.
4. Extract from, digest, and store the Retained EPUB. ADR 0035's identity
   resolution then applies to it unchanged, except for the idempotency rule
   below.

ADR 0035 step 2 therefore reads: download and validate the complete Catalog
file, derive the Retained EPUB, then compute the versioned digest of the
Retained EPUB's container bytes. ADR 0103's "a content revision still owns
original bytes/digest" means the Retained EPUB's bytes and digest. Structure
checks and source structure updates re-extract the Retained EPUB, which keeps
every resource extraction reads.

### Slimming rules (version 1)

- Remove manifest items whose media type is an image, font, audio or video,
  together with their container entries. Removal is driven by the manifest
  only; entries it does not list are kept.
- Never remove `mimetype`, anything under `META-INF/`, a package document, any
  spine item (whatever its media type), the navigation document or NCX, or a
  stylesheet.
- In the package document, cut only the removed `<item>` elements' byte
  ranges. Leave dangling references from XHTML, the cover `<meta>` or the guide
  as they are.
- If the package document cannot be parsed unambiguously, fail acquisition as
  an invalid EPUB rather than guess.
- The output must be deterministic: a pure function of the Catalog file's bytes
  and the slimming version. Kept entries are copied raw, in their original
  order. A rewritten package document keeps its entry's position, name, method
  and modification time.
- When nothing matches, the Retained EPUB is the Catalog file, byte-for-byte.

### Identity and provenance

- Digest version stays 1: the stored digest is SHA-256 of the stored Retained
  EPUB, so ADR 0035's invariant and constraints need no change.
- The acquisition lookup that makes repeated downloads idempotent matches a
  content revision whose digest equals either the Retained EPUB's or the
  Catalog file's. The second matches revisions stored before this ADR, which
  hold the unslimmed Catalog file. Re-acquiring them is not a content change.
- The `epub.import` processing history records the Catalog file's digest and
  size, the Retained EPUB's size, the slimming version and the removed items.
  No schema change.
- A later change to the slimming rules gets a new version. It applies to new
  acquisitions only and never re-digests existing content revisions.

### Existing content

Content revisions stored before this ADR keep their bytes and digests. Slimming
them would change digests and make analyses stale, so a backfill needs its own
decision.

## Consequences

- Illustrated editions up to 512 MiB are acquirable, and their stored size
  follows their text. Acquisition's peak memory is bounded by the Retained
  EPUB, not the download.
- Clean EPUBs are stored and digested exactly as before.
- The stored digest no longer identifies the catalog's file for slimmed
  revisions. Processing history keeps that link.
- A Retained EPUB is not a faithful copy of the Catalog file: it is missing its
  images and fonts. Nothing in Mouseion reads them today. A future feature that
  needs them, such as rendering illustrations, has to re-acquire.
- Acquisition stays synchronous inside the request. EPUB bodies get their own
  deadline instead of the 30-second feed timeout, and acquisition must finish
  even when the response deadline passes.
- Temporary disk space up to 512 MiB per concurrent acquisition replaces the
  equivalent memory.

## Alternatives considered

- **Raise the limit only.** Holds hundreds of MiB per request in memory and
  stores illustrations Mouseion never reads.
- **Stream only.** Extraction and `bytea` storage still need the whole package
  in memory, so peak memory barely changes.
- **Store the Catalog file and slim on read.** Keeps the storage cost and moves
  the work into every structure check.
- **Store both files.** Doubles storage for the files this is meant to shrink.
- **Store the Catalog file's digest in a new column.** Processing history
  already carries it, and nothing would query the column.
- **Rewrite XHTML to drop `<img>` tags or inline data.** Changes extraction
  inputs and offsets for no storage gain worth the risk.
- **Move EPUB storage out of Postgres.** A larger schema and operations change
  that this problem does not require.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived](0047-acquisition-folded-into-analysis.md)
- [ADR 0103: Version source structure without rewriting frozen evidence](0103-source-structure-and-poetry-evidence.md)
- [#2276: Oversized OPDS EPUB is reported as a connection failure](https://github.com/justin-hayes/mouseion/issues/2276)
