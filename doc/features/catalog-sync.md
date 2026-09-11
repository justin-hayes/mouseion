# Catalog Sync

Status: Implemented · Date: 2026-09-02 · Updated: 2026-09-11

## Motivation

A learner may already curate a large collection in Calibre and expose it through
Calibre-Web OPDS. Adding those books one at a time makes My Books incomplete and
turns catalog maintenance into repetitive work. Mouseion should recognize the
studyable part of that collection while keeping sync metadata-only. Acquisition
and analysis happen only when learner intent is expressed by adding the Book to
Reading Journey, as defined by [ADR 0049](../adr/0049-reading-intent-triggers-analysis.md)
and the later standalone-action retirement in [ADR 0054](../adr/0054-retire-standalone-analysis-action.md).

## Goal

Keep bibliographic metadata from each learner-owned catalog connection
reconciled into My Books for every non-English language the catalog offers
whose NLP pipeline is ready, without downloading EPUBs or deleting
learner-owned state. The chosen-language Books produced by this process derive
the learner's study-language set.

## Scope

This feature covers automated per-connection metadata sync, explicit **Sync
now**, per-book metadata refresh, connection-level status, and the transition
from a metadata-only Book to per-book content acquisition through Reading
Journey intent. My Books is the sole browse surface for the synced collection.
Its metadata-first and non-destructive behavior is governed by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md); its capability-driven language scope is reconciled by [ADR 0043](../adr/0043-study-languages-derived-settings-removed.md).

## Requirements

### Fresh-account entry

- When My Books is empty and the learner has no catalog connections, explain
  that Mouseion needs a learner-owned catalog connection to synchronize the
  ready-language collection.
- The primary empty-state action opens the Catalogs destination, where the
  learner adds a catalog connection. It does not silently create a connection.
- Books enter My Books through catalog synchronization; there is no manual
  metadata-entry path.

### Connection configuration and status

- `/catalogs` (with the legacy `/connections` redirect) is the learner-owned
  configuration and maintenance destination for name, URL, username, encrypted
  credential, sync, edit, and delete behavior. Catalog maintenance is a
  principal destination per
  [ADR 0058](../adr/0058-catalog-maintenance-principal-destination.md).
- Each connection distinguishes **Never synced**, **Syncing**, **Last synced**,
  and **Sync failed**. Last-synced information belongs to the connection, not a
  global dashboard.
- **Sync now** enqueues the same River work used by the periodic schedule. It
  must not create a parallel execution contract.
- A failure names the connection, preserves prior My Books data, and offers edit
  connection or retry as appropriate.
- Detailed attempts, retry, cancellation, and errors remain operationally
  visible through `/jobs`.

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
- It upserts title, language, and catalog identity through the owner-scoped
  Book alias and duplicate rules in ADR 0035.
- A new match becomes an active metadata-only My Books entry. No placeholder
  source, empty content record, scope, analysis, or deck is created.
- Repeated runs are idempotent. Metadata changes propagate without duplicating
  Books or membership.
- Entries removed upstream remain in My Books. Deleting a connection also
  leaves all Books, membership, acquired content, and derived history intact.

### Lazy content acquisition and per-book refresh

- Sync never downloads or re-downloads EPUB content.
- Metadata-only Books have no detail page. **Add to Reading Journey** acquires,
  validates, and analyzes the EPUB in one ensure-once flow; the My Books row
  remains the place to refresh metadata or remove the Book.
- Acquired state is published only after complete EPUB download, validation,
  and immutable snapshot persistence, as required by ADR 0035.
- Each catalog-backed Book offers a metadata refresh for that entry. Refresh
  is an idempotent metadata-only upsert and has no scope or analysis effect.
- If the individual upstream entry is no longer present, refresh is a calm
  no-op that preserves the Book and its current metadata.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No connections and empty My Books | Explain why a connection is needed and that sync records metadata before content. | Add catalog connection |
| Never synced | Identify the connection and explain that every offered non-English language with a ready NLP pipeline is eligible. | Sync now |
| Syncing | Preserve existing collection and show that metadata reconciliation is operational work. | View operational status |
| Last synced | Show the last successful time and metadata-only reconciliation, with ordinary edit/delete actions. Do not present a per-connection language-scope summary. | Sync now or My Books |
| Sync failed | Name the connection, preserve prior data, and show an actionable reason. | Edit connection or retry |
| Metadata-only Book | Identify that content is not yet acquired and that analysis is unavailable until it is. | Add to Reading Journey |
| Acquisition/analysis running or failed | Preserve Book or Journey context and distinguish durable content/analysis work. | View status or retry |
| Current analysis stale | Identify that the acquired content changed since the current evidence was produced. | Re-analyze from Reading Journey |
| Individual metadata refresh complete | Show refreshed metadata/last-refreshed state without implying content changed. | Return to My Books |

## Non-goals

- Downloading all content during sync or re-downloading content on resync.
- Detecting changed EPUB bytes or invalidating current analysis from sync.
- Destructively reconciling upstream removals.
- Synchronizing English, languages whose NLP pipeline is not ready, or languages
  the connected catalog does not expose.
- A global sync dashboard or an upstream catalog browser. Catalog maintenance is
  a principal destination per
  [ADR 0058](../adr/0058-catalog-maintenance-principal-destination.md); a global
  sync dashboard is not.
- Batch-selecting Books for analysis; whether a future batch contract exists is
  an open product question.
- Reintroducing an administrator role or administrator-managed catalogs.
- Resolving the default cadence, per-connection cadence configuration,
  last-synced storage shape, or exact lazy-acquisition trigger in this document.

## Acceptance criteria

- A fresh learner with no connection sees a clear My Books explanation and a
  primary action to the Catalogs destination.
- A connection can be never synced, syncing, last synced, or failed, and every
  state has a usable server-rendered path and recovery where applicable.
- **Sync now** and periodic execution invoke the same owner-scoped,
  idempotent reconciliation.
- Every offered non-English language with a ready NLP pipeline is walked; no
  saved study-language preference gates the run.
- Repeated runs create no duplicates; upstream metadata changes update local
  metadata; upstream removal and connection deletion remove nothing from My
  Books.
- Sync performs no EPUB download and causes no scope or analysis invalidation.
- Per-book refresh updates metadata only and safely no-ops when the entry is
  missing.
- Operational jobs remain visible and recoverable through `/jobs` without
  changing primary navigation.
