# Catalog Sync

Status: Implemented, including independent cover retrieval · Date: 2026-09-02 · Updated: 2026-11-22

## Motivation

A learner may already curate a large collection in Calibre and expose it through
Calibre-Web OPDS. Adding those books one at a time makes My Books incomplete and
turns catalog maintenance into repetitive work. Mouseion should recognize the
studyable part of that collection while keeping sync metadata-only. Acquisition
and analysis happen only when learner intent is expressed by moving the Book to
To Read, as defined by [ADR 0049](../adr/0049-reading-intent-triggers-analysis.md)
and the later standalone-action retirement in [ADR 0054](../adr/0054-retire-standalone-analysis-action.md).

## Goal

Keep a local Catalog index current for each learner-owned connection in every
non-English language the catalog offers whose NLP pipeline is ready, without
downloading EPUBs, creating Books, or deleting learner-owned state. Books the
learner adds from that index keep their title, author and cover reconciled to
the entry; only chosen Books derive the learner's study-language set.

## Scope

This feature covers automated per-connection metadata sync, explicit **Sync
now**, connection-level status, and the transition from a Catalog entry to a
chosen Book and content acquisition through Reading intent. Sync never creates
a Book, membership, or disposition, and the per-Book metadata refresh action is
retired. The **Catalog** at `/catalogs` browses the Classical catalog and
each connection's local Catalog index; My Books remains the learner's
bibliographic Book surface. Its metadata-first and non-destructive behavior is governed by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md); its capability-driven language scope is reconciled by [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md).

## Requirements

### Fresh-account entry

- When My Books is empty and the learner has no catalog connections, explain
  that Mouseion needs a learner-owned catalog connection to synchronize the
  ready-language collection.
- The primary empty-state action enters the Catalog at `/catalogs`. Its OPDS
  area offers **Connect an OPDS catalog**, which leads to **Manage sources** at
  `/catalogs/sources`; it does not silently create a connection.
- Books enter My Books only when the learner chooses a Catalog entry; sync and
  browsing never create them, and there is no manual metadata-entry path.

### Connection configuration and status

- **Manage sources** at `/catalogs/sources` is the learner-owned configuration
  and maintenance surface for name, URL, username, encrypted credential, sync,
  edit, and delete behavior. Connection mutations land there.
- The **Catalog** at `/catalogs` shows one status line per connection with
  **Sync now**, above the Classical catalog. `GET /connections` permanently
  redirects to `/catalogs` while preserving supported deep-link parameters, and
  the legacy `POST /connections` mutation routes remain supported.
- Each connection distinguishes **Never synced**, **Syncing**, **Last synced**,
  and **Sync failed**. Last-synced information belongs to the connection, not a
  global dashboard.
- **Sync now** enqueues the same River work used by the periodic schedule. It
  must not create a parallel execution contract.
- A failure names the connection, preserves prior My Books data, and links to
  Manage sources, where the learner can edit the connection or retry as
  appropriate.
- Detailed attempts, retry, cancellation, and errors remain operationally
  visible through `/jobs`.

### Catalog browsing

- The Catalog at `/catalogs` has one search and one language filter over the
  Classical catalog and every connection's local Catalog index. The filter
  defaults to the Last study language, is never stored, and is all languages
  when the learner has none.
- Each connection appears as a section whose entries are a cover grid with
  cover, title and author, ordered by title then author and paged locally.
- Search compares the Go search key for title and author, so it ignores accents
  and case. Classical search also matches the Greek title. Search and browsing
  never contact the OPDS server.
- An entry linked to one of the learner's active Books shows **In My Books**
  linked to that Book and no disposition, analysis or preparation state. An
  unowned entry offers **Add** and the secondary **I've read this**.
- **Add** creates the Book linked by its catalog-entry alias, or restores the
  learner's existing Book for that alias, as To Read. It then runs the same
  reading-intent preparation seam as **Read again**: acquisition and analysis
  start once, and repeating Add changes nothing.
  The Add copy says the text is downloaded and analyzed, the route is an
  ordinary owner-scoped POST, and the confirmation names the Book and links to
  My Books. Adding the first Book in a language makes it a study language.
- **I've read this** creates or restores the same Book as Read with a
  **Previously read** record. It downloads nothing, starts no analysis or
  preparation and changes no Known vocabulary. On a classical Work it is offered
  once per Edition and covers the whole Work. An entry whose Book left My
  Books restores that same Book as Read.
- Distinct empty states cover no OPDS sources, no match, nothing in this
  language, and nothing to browse yet. One live region announces result
  updates; search, filter, paging, ownership links and Add work without
  JavaScript.

### Language scope

- A run walks every non-English language the catalog offers whose NLP pipeline
  is currently advertised as ready by the service.
- English is always excluded under the current product assumption that it is
  every learner's native language.
- A catalog language that is not offered or not ready is not synchronized.
- Study languages are derived from active Books whose language is chosen; there
  is no saved study-language preference gating the run. The learner's active
  study language ([ADR 0050](../adr/0050-active-study-language.md)) is a stored
  context pointing into that derived set — it never defines which languages sync
  walks. Changing a Book's language state does not delete its membership,
  acquired content, analyses, decks, or vocabulary.

### Metadata-first reconciliation

- Sync walks the applicable Calibre-Web language feeds using the connection's
  existing origin-scoped OPDS client and bounded pagination.
- It maintains the connection's **Catalog index**: a local mirror of the
  entries the connection offers within the sync scope, keyed by connection and
  entry identifier. Each entry records title, author, language tag, advertised
  cover provenance and the EPUB acquisition link, plus when the entry was last
  seen. Acquisition reads that stored link instead of re-walking a language
  feed. The index holds no learner state. A complete run deletes entries it no
  longer saw; a partial or failed run deletes nothing; deleting the connection
  deletes its index.
- It never creates a Book, activates membership, writes a disposition, or
  changes a Book's language. An unowned entry exists only in the Catalog index.
- For an entry linked to one of the learner's Books by catalog-entry alias, it
  updates that Book's title, author, and advertised cover. Language, identity,
  membership, and disposition stay as the learner's Add left them.
- Repeated runs are idempotent. Metadata changes propagate without duplicating
  Books, membership, or dispositions.
- A Book that left My Books is not put back by sync. Entries removed upstream
  leave the index; chosen Books and their metadata remain. Deleting a connection
  also leaves all Books, membership, acquired content, and derived history
  intact.

### Accepted cover-metadata extension

[Book Covers](book-covers.md) and
[ADR 0077](../adr/0077-catalog-supplied-book-covers.md) extend reconciliation
with catalog-advertised image provenance and independent bounded retrieval. Sync
does not wait for optional image work and cover failure does not fail metadata
reconciliation. Retained normalized images remain available while the catalog is
offline and survive connection deletion; catalog URLs and credentials are never
sent to the browser.

"Metadata-first" continues to mean that sync never downloads EPUB content,
creates acquired source evidence, or triggers analysis; it does not prohibit the
shipped independent retrieval of optional catalog-supplied display metadata.

### Lazy content acquisition

- Sync never downloads or re-downloads EPUB content.
- A Book chosen from the Catalog has no detail page. **Add**, **Read again**
  and **Retry** enqueue the durable `epub_acquisition` job and return at
  once with one confirmation that the Book is being downloaded and analyzed.
  The worker resolves the EPUB acquisition link stored in the Catalog index and
  never walks the connection's language feeds. The Book's metadata follows the
  next sync.
- Add on a Catalog entry expresses the same intent: it creates or restores the
  Book as To Read and then uses the same acquisition route. A failure never
  appears in the Add response; it stays on the Book as its recorded reason, with
  Retry on My Books.
- When acquisition cannot resolve the Book's catalog entry at all — the entry
  has left the index, the Book has no catalog-entry alias, or the alias's
  connection was deleted — the worker records stage `entry` and code
  `catalog_entry_missing`. A missing index entry keeps the connection and entry
  identifier on the failure; a missing alias or connection records no catalog
  identity, so the failure survives the connection's deletion. The Book stays
  To Read and My Books shows the reason with Retry. A later sync that restores
  the entry makes Retry acquire it without any other change.
- Acquired state is published only after complete EPUB download, validation,
  and immutable snapshot persistence, as required by ADR 0035. The worker then
  submits the existing analysis job unchanged.
- No Book creation or membership activation leaves an unchosen member in My
  Books (ADR 0117). Each yields a To Read Book (or a
  Read Book, for "I've read this"). EPUB ingest that matches a Book by its
  source identifier and finds it removed from My Books restores it as To Read,
  exactly as Add does; its analysis, history and Known vocabulary stay. A Book
  that is already an active member keeps its disposition. EPUB ingest that
  matches no Book creates one as To Read. A legacy row (active, not To Read, no
  reading history) is never produced; if one exists, the acquisition and
  analysis workers skip it, and nothing re-activates it.
- There is no per-Book metadata refresh action or route. A retired
  `/library/books/{id}/refresh` request returns 404. If an entry is no longer
  present in a complete sync, the Book keeps its current metadata.

### EPUB acquisition failures

A failed EPUB acquisition never rolls back the learner's To Read choice.
There is no separate Inbox step to undo: the Add commits first, and acquisition
runs as the durable `epub_acquisition` job (ADR 0121). The worker records the
per-Book failure after the attempt and deletes it when an acquisition succeeds,
so My Books keeps showing the reason until recovery. Add, Read again and
Retry never report the failure in their responses.

The stages and stable codes are defined once in the domain:

| Stage | Code | Retry |
|---|---|---|
| `entry` | `catalog_entry_missing` | Yes, after a sync restores the entry |
| `fetch` | `source_download_failed` | Yes |
| `source` | `source_oversize`, `source_format_unknown` | No |
| `import` | `epub_import_failed` | Yes |

My Books renders the reason and, where retrying can help, **Retry**, which goes
through reading intent's Retry. A source-stage refusal offers no Retry because
the file itself cannot be acquired. Book evidence classification takes the
failure as an input beside Classical preparation's, and a failure never
overrides an analysis already in effect. The operator log line for the failed
attempt remains.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No connections and empty My Books | Explain why a connection is needed and that sync records metadata before content. | Add catalog connection |
| Never synced | Identify the connection and explain that every offered non-English language with a ready NLP pipeline is eligible. | Sync now |
| Syncing | Preserve existing collection and show that metadata reconciliation is operational work. | View operational status |
| Last synced | Show the last successful time and metadata-only reconciliation. Manage sources keeps the ordinary edit/delete actions. Do not present a per-connection language-scope summary. | Sync now or My Books |
| Sync failed | Name the connection, preserve prior data, and show an actionable reason linked to Manage sources. | Edit connection in Manage sources or retry |
| Metadata-only Book | Identify that content is not yet acquired and that analysis is unavailable until it is. | View in Reading or Read again |
| Read outside Mouseion | Record that the learner read the text before using Mouseion, with no download, analysis or vocabulary change. | I've read this |
| EPUB acquisition failed | Keep the Book To Read, name the reason (entry missing, download failed, unusable source, or import failed), and offer Retry only for entry, fetch and import stages. The reason stays until an acquisition succeeds and never overrides an analysis in effect. | Retry, or fix the catalog and retry |
| Acquisition/analysis running or failed | Preserve Book or Journey context and distinguish durable content/analysis work. | View status or retry |
| Current analysis stale | Identify that the acquired content changed since the current evidence was produced. | Retry from My Books or Reading |

## Non-goals

- Downloading all content during sync or re-downloading content on resync.
- Detecting changed EPUB bytes or invalidating current analysis from sync.
- Destructively reconciling upstream removals.
- Synchronizing English, languages whose NLP pipeline is not ready, or languages
  the connected catalogue does not expose.
- Adding a global sync dashboard or an upstream catalog browser. Catalog is a
  primary destination for browsing and source status; Manage sources owns
  connection setup and sync maintenance.
- Batch-selecting Books for analysis; whether a future batch contract exists is
  an open product question.
- Reintroducing an administrator role or administrator-managed catalogs.
- Resolving the default cadence, per-connection cadence configuration,
  last-synced storage shape, or exact lazy-acquisition trigger in this document.

## Acceptance criteria

- A fresh learner with no connection sees a clear My Books explanation and a
  primary action to `/catalogs`.
- A connection can be never synced, syncing, last synced, or failed, and every
  state has a usable server-rendered path and recovery where applicable.
- **Sync now** and periodic execution invoke the same owner-scoped,
  idempotent reconciliation.
- Every offered non-English language with a ready NLP pipeline is walked; no
  saved study-language preference gates the run.
- Repeated runs create no duplicates; upstream metadata changes update the
  linked Book's title, author, and cover; upstream removal and connection
  deletion remove nothing from My Books.
- A fresh learner's My Books stays empty after a sync, and study languages come
  only from Books they added.
- Sync performs no EPUB download and causes no scope or analysis invalidation.
- Sync updates the title, author, and cover of Books linked to an entry and
  never creates a Book, membership, or disposition.
- A Book that left My Books is not restored by sync; the per-Book metadata
  refresh action and route are gone.
- Operational jobs remain visible and recoverable through `/jobs` without
  changing primary navigation.
