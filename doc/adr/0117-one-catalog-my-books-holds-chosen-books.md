# ADR 0117: One Catalog for every source; My Books holds only chosen Books

Status: **Implemented; per-source Catalog presentation amended by [ADR 0129](0129-one-catalog-presentation-for-every-source.md)** · Date: 2026-11-21 · Author: Justin + OpenCode

Amended 2026-11-21 by [issue #2505](https://github.com/justin-hayes/mouseion/issues/2505): a failed EPUB acquisition is recorded as Book evidence ("A failed EPUB acquisition is Book evidence"), which adds ADR 0047 to the ADRs this one amends.

Amended 2026-11-22 by [ADR 0121](0121-book-preparation-status-vocabulary-and-durable-epub-acquisition.md): the deferred alternative "Acquire the EPUB in a durable job" is adopted, so "Acquisition still runs in the request" no longer holds, and the EPUB acquisition failure becomes one kind of Book preparation failure.

Amended 2026-11-24 by [ADR 0128](0128-a-classical-book-is-a-whole-work.md): a classical Work is one Book per Edition, so Adding a Work queues one preparation, **I've read this** is offered once per Edition rather than per Reading unit, and the grouped Work row's **Remove the work** is retired in favour of the ordinary Remove.

Amends [ADR 0035](0035-my-books-membership-and-source-provenance.md),
[ADR 0041](0041-catalog-sync-metadata-first.md),
[ADR 0047](0047-acquisition-folded-into-analysis.md),
[ADR 0058](0058-catalog-maintenance-principal-destination.md),
[ADR 0078](0078-book-dispositions-and-current-reading.md),
[ADR 0086](0086-reading-working-desk-hidden-visibility-and-concordance.md) and
[ADR 0116](0116-classical-catalog.md). Decided in a grilling session on
2026-11-21; the spec,
[#2488](https://github.com/justin-hayes/mouseion/issues/2488), holds the full
behaviour.

## Context

Two rules now split how a learner finds and adds a Book.

- OPDS sync turns every eligible catalog entry into a Book with Inbox
  disposition. My Books is the only place to browse those entries (ADR 0041, ADR 0058).
- The Classical catalog is browsed on Catalogs, and Add creates an Inbox Book
  that downloads nothing (ADR 0116). ADR 0116 had to amend ADR 0058 into
  "Catalogs browses only curated sources that do not sync", which is a boundary
  defined by mechanism, not by what the learner is doing.

As a result, the Inbox means two things: untriaged synced offerings and
deliberately chosen classical texts. My Books mixes the learner's
collection with an upstream mirror, and Hidden exists mostly to remove synced
clutter. Study languages are derived from Books, so they include languages the
learner only ever synced.

## Decision

### A catalog entry is not a Book

A Book exists only once the learner chooses it. Catalog entries from every
source are browsed on one surface, and nothing becomes a Book by being synced
or listed. The glossary's "a Book is always in My Books once discovered" no longer holds.

### Sync maintains a local catalog index, not My Books

Each OPDS connection keeps its periodic, metadata-only sync and **Sync now**.
The sync scope is unchanged: non-English languages whose NLP pipeline is ready.
Sync now writes a per-connection **catalog index** keyed by (connection, entry
identifier), the identity ADR 0044 already uses for aliases. The index holds
title, author, language, cover and the EPUB acquisition link, so acquisition no
longer re-walks the language feed. The index mirrors upstream: entries removed
upstream leave the index, and deleting a connection deletes its index. The
index is not learner state.

A Book linked to an index entry takes that entry's metadata at each sync. The
per-Book metadata refresh action is retired. Browsing and search are local and
never contact the OPDS server; the unused live OPDS search in the client stays
unused.

### Catalog is the single browse-and-add surface

The **Catalogs** destination is relabelled **Catalog** and keeps `/catalogs`
and its redirects. It has one search and one language filter over every
source; the filter defaults to the active study language when there is one,
otherwise all languages. Results are grouped by source:

- The Classical catalog keeps its author-grouped index, Openings, length strips
  and Work pages.
- Each OPDS connection shows a paged cover grid, ordered by title then author.

The catalog shows only **Add** or **In My Books** per entry, never disposition,
analysis or preparation state. Entries are not deduplicated across sources.
Connection management moves to `/catalogs/sources`; the Catalog page shows each
connection's sync status line, with Sync now and its failure message.

ADR 0058's "My Books is the sole browse surface for synced metadata" and ADR 0116's
"Catalogs browses only curated sources that do not sync" are both replaced by
this rule.

### Add means To Read; Remove is its opposite

**Add** creates the Book, or restores an earlier Book for the same entry or
Reading unit, as To Read. It triggers exactly what To Read triggers today:
EPUB acquisition and analysis, or classical preparation for each Reading unit.
Adding a whole Work queues one preparation per Reading unit on
`classical_preparation`. ADR 0116's "Adding downloads nothing" is superseded.

**I've read this**, a secondary catalog action, creates the Book as Read with a
Previously read record. It downloads nothing and changes no vocabulary. For a
Work it is offered per Reading unit only.

**Remove from My Books** ends membership and deletes nothing. The Book's
identity, analysis, corrections, exclusions, completions and Known vocabulary
remain, and adding the entry again restores the same Book. The current Book
cannot be removed. A grouped Work row offers **Remove the work** in place of
"Move the work back to Inbox".

### Every Book in My Books is Currently reading, To Read or Read

The Inbox is retired as a learner-facing state. Disposition reduces to whether
the Book is To Read. Finishing a reading or marking a Book previously read
clears it, and the Book shows as Read through its history. **Read again** sets
it. Ending a current reading without finishing leaves the Book To Read. My
Books has two tabs: **To Read** (including the Currently reading Book) and
**Read**. Its empty state leads to Catalog.

**Hidden** is retired, together with Show hidden books and the Work-level Hide
and Unhide; Remove replaces it. The **needs language** strip and
`/library?needs-language` are retired. Study languages are still derived from
Books in My Books, which now means only chosen Books.

### Existing Inbox Books are removed, not promoted

A separate data backfill under ADR 0038 removes from My Books every Book that is
Inbox with no reading history, including classical Books that were added but
never moved to To Read, as well as unread Books without a language. It only
changes membership, is idempotent and can be reversed by restoring membership.
It runs only after every connection's index has completed a sync, so no
Book leaves My Books before its entry is browsable. A language-less Book with
reading history is also removed, because My Books is browsed within a study
language and would list it nowhere (#2575); its completions are kept, the
backfill reports it, and adding it again with a language restores it. Retired `book_visibility` data is left
in place for a separately reviewed cleanup.

### A failed EPUB acquisition is Book evidence

Add creates a Book before its EPUB is acquired, so a failure that is only
reported in the response would leave a To Read Book with no visible reason and
no recovery. Acquisition still runs in the request, as ADR 0047 decided, but its
failure is now recorded per Book with a stable stage and code, and cleared by
the next successful acquisition. The stages and codes reuse Classical
preparation's vocabulary where the meaning matches; the one new stage, `entry`,
covers a catalog entry that has left the index. The domain classifies both
failure kinds when it classifies Book evidence (ADR 0052), and My Books shows the
reason with Retry where retrying can help. This amends ADR 0047's "On failure
the request fails inline with the existing OPDS error messaging".

## Alternatives considered

- **Keep sync into My Books and add OPDS browsing to Catalogs.** Rejected: the
  same offering would appear in two places, and the Inbox would keep both of its
  meanings.
- **Live OPDS browse and search with no sync.** Rejected: browsing would depend
  on the Calibre-Web server being reachable, search could not be uniform with the
  local Classical catalog, and it would bring back the retired upstream browser.
- **Add creates a saved Book with no status.** Rejected: that is the Inbox
  under another name.
- **Move existing Inbox Books to To Read.** Rejected: it would acquire and
  analyze the whole synced collection at once, which ADR 0041 refuses.
- **Merge duplicate entries across sources.** Rejected: ADR 0035/0044
  identity rules already decide Book identity, and with the expected single
  connection duplicates do not arise.
- **Acquire the EPUB in a durable job, as Classical preparation does.**
  Deferred, not rejected: it would give both sources one shape, but it reopens
  ADR 0047 and belongs with a shared status vocabulary for analysis and
  Classical preparation. One Add acquires one EPUB, as Move to To Read does
  today, and a recorded failure is enough for recovery.
- **Keep acquisition failures in the response only.** Rejected: with Add
  creating the Book, a failed first acquisition would strand it.

## Consequences

- A new learner has no study language until the first Add. Surfaces scoped to
  the active study language rely on the existing "no study language" handling.
- Adding becomes a costly action: it downloads and analyzes. The catalog's Add
  copy must say so.
- The catalog index needs a migration (human review under CODEOWNERS). The
  Inbox backfill is a second, data-only migration or job, reviewed separately.
- Recording EPUB acquisition failures needs a second additive migration (human
  review under CODEOWNERS).
- Delivery in three slices:
  1. the index, its migration, and sync writing to it alongside today's sync, with no
     learner-visible change; then recorded EPUB acquisition failures with Retry
     on My Books, which also fixes today's Move to To Read;
  2. the unified Catalog page, Add as To Read, I've read this, and Remove;
  3. retiring the Inbox, Hidden and needs-language features from My Books, plus the
     backfill.
- `CONTEXT.md`, `doc/product.md`, `doc/design/information-architecture.md`,
  `doc/design/screen-inventory.md`, `doc/design/terminology.md`,
  `doc/design/components.md`, `doc/features/catalog-sync.md`,
  `doc/features/collection-browsing.md` and `doc/features/book-covers.md` are
  reconciled as the slices ship.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0044: Catalogue-entry alias identity retains the catalogue connection](0044-catalogue-entry-connection-scoped-identity.md)
- [ADR 0047: Content acquisition is folded into analysis](0047-acquisition-folded-into-analysis.md)
- [ADR 0058: Catalog maintenance is a principal destination](0058-catalog-maintenance-principal-destination.md)
- [ADR 0078: Book dispositions and current reading](0078-book-dispositions-and-current-reading.md)
- [ADR 0086: Reading Working desk, independent Hidden visibility, and corpus-wide Concordance](0086-reading-working-desk-hidden-visibility-and-concordance.md)
- [ADR 0116: A curated Classical catalog of Ancient Greek Reading units](0116-classical-catalog.md)
