# ADR 0142: Record the Catalog file's Document digest

Status: **Proposed; needs human review** · Date: 2026-11-28 ·
Author: Justin + OpenCode

Specified in [#2865](https://github.com/justin-hayes/mouseion/issues/2865);
implemented by [#2866](https://github.com/justin-hayes/mouseion/issues/2866)
(record it at acquisition) and
[#2867](https://github.com/justin-hayes/mouseion/issues/2867) (backfill existing
revisions). Amends
[ADR 0109](0109-retained-epub-from-catalog-acquisition.md) and reverses its
rejected alternative "Store the Catalog file's digest in a new column". Leaves
[ADR 0035](0035-my-books-membership-and-source-provenance.md)'s digest-version-1
identity rules and [ADR 0103](0103-source-structure-and-poetry-evidence.md)
unchanged.

## Context

The learner reads their Books in KOReader on an e-reader. KOReader identifies
every document by its **Document digest**, a partial MD5 of the file's bytes,
and reports it in progress sync and reading statistics. Since ADR 0109
Mouseion stores the Retained EPUB, which is the Catalog file without images,
fonts, audio and video, and keeps only the Catalog file's SHA-256. A partial MD5
cannot be derived from a SHA-256, and cannot be derived from the Retained EPUB
when slimming changed the file. Mouseion therefore cannot tell which Book a
KOReader digest belongs to, even though the learner may have downloaded that
very file from the same OPDS catalog.

## Decision

- **Acquisition records the Catalog file's Document digest** on the content
  revision it creates, and in the `epub.import` processing history beside
  `catalog_digest`. The digest is the partial MD5 of the Catalog file as served,
  never of the Retained EPUB.
- **The algorithm is KOReader's `util.partialMD5`.** MD5 over 1024-byte samples
  at offsets 0, 1 KiB, 4 KiB, 16 KiB and so on by factors of four up to 1 GiB
  (`1024 << 2i` for i from -1 to 10, where i = -1 gives 0 under LuaJIT's masked
  shift). It stops at the first offset at or past the end of the file; a sample
  cut short by the end contributes the bytes that exist. The result is 32
  lower-case hex digits. A streaming writer computes it while the download
  streams, beside the SHA-256 hasher, at no extra memory or time, and is checked
  against KOReader's own test vectors.
- **It is stored as a nullable `document_digest` column on
  `source_content_revisions`**, checked to 32 lower-case hex digits when present
  and indexed on owner and digest. It is on the revision because the revision is
  what derives from a Catalog file, so a Book with several revisions matches
  through any of them. It is not unique: two revisions may share a Catalog file,
  and one digest can stand for several Books on a device.
- **Revisions stay immutable in content.** The immutability trigger now permits
  exactly one change: `document_digest` going from NULL to a value, with every
  other column unchanged. A recorded digest is never overwritten or cleared. The
  trigger gets its own function; the snapshot and unit tables keep the original
  one and stay fully immutable. ADR 0035 and ADR 0103, which rely on revision
  immutability, are unaffected.
- **A repeat acquisition fills a missing digest.** When the idempotent lookup
  matches a revision, by Retained EPUB or Catalog file SHA-256, that has no
  Document digest, acquisition records it on that revision. It creates no new
  revision and never overwrites a different digest. A direct EPUB import records
  the digest of the bytes it stores.
- **A separate data-only migration backfills** revisions whose stored bytes are
  the Catalog file (#2867). Revisions slimming changed are left empty rather than
  given a wrong digest.
- **Nothing reads the column yet.** Document links, Unlinked documents and
  ambiguity are decided with the lookup by digest in a later spec. A digest is
  not a unique key.

## Alternatives considered

- **Keep the Catalog file.** Defeats ADR 0109's storage saving.
- **Serve the Retained EPUB to the device.** The device would read without the
  images, fonts or cover.
- **Keep only the SHA-256.** A partial MD5 cannot be derived from it.
- **A side table keyed by revision.** Avoids touching the trigger but separates
  a revision's identity from the revision for no gain, since the narrowed trigger
  keeps the content immutable.
- **Disable the trigger during the backfill.** A hand-run step the migration
  cannot own.

## Consequences

- Books acquired from now on, and after the backfill most Books acquired before,
  can be matched to a KOReader document without the learner linking it by hand.
- A slimmed revision acquired before this ADR has no digest until it is
  re-acquired.
- A pass-through OPDS proxy of the Catalog file would make matches deterministic
  for slimmed Books too. It is a possible later option and out of scope here.
- Clearing a digest, which recovery from a bad backfill needs, means replacing
  the trigger function temporarily in a hand-run transaction.
- Needs human review under CODEOWNERS (ADR, migration).
