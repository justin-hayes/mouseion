# Catalogue Sync

Status: Proposed · Date: 2026-09-02

## Motivation

A learner may already curate a large collection in Calibre and expose it through
Calibre-Web OPDS. Adding those books one at a time makes My Books incomplete and
turns catalogue maintenance into repetitive work. Mouseion should recognize the
studyable part of that collection while preserving deliberate content
acquisition and the explicit scope-and-analysis lifecycle.

## Goal

Keep bibliographic metadata from each learner-owned catalogue connection
reconciled into My Books for the learner's ready study languages, excluding
English, without downloading EPUBs or deleting learner-owned state.

## Scope

This feature covers automated per-connection metadata sync, explicit **Sync
now**, per-book metadata refresh, connection-level status, and the transition
from a metadata-only Book to lazy content acquisition. It complements the
selective manual acquisition path in
[Explicit Scoped-Analysis Workflow](explicit-scoped-analysis-workflow.md) and is
governed by [ADR 0041](../adr/0041-catalog-sync-metadata-first.md).

## Requirements

### Fresh-account entry

- When My Books is empty and the learner has no catalogue connections, explain
  that Mouseion needs a learner-owned catalogue connection to synchronize the
  ready-language collection.
- The primary empty-state action adds a catalogue connection at `/connections`.
  It enters the existing **Add books** workflow; it does not add a destination
  or silently create a connection.
- Manual metadata entry and selective catalogue browsing remain possible where
  already supported, without competing with the primary onboarding action.

### Connection configuration and status

- `/connections` remains the learner-owned configuration and maintenance
  surface for name, URL, username, encrypted credential, browse, edit, and
  delete behavior.
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

- A run includes only the intersection of the learner's saved study languages
  and languages currently advertised as ready by the NLP service.
- English is always removed from that intersection under the current product
  assumption that it is every learner's native language.
- A catalogue language that is not saved or not ready is not synchronized.
- Removing a study-language preference never deletes already synchronized
  metadata-only Books, acquired content, analyses, decks, or vocabulary.

### Metadata-first reconciliation

- Sync walks the applicable Calibre-Web language feeds using the connection's
  existing origin-scoped OPDS client and bounded pagination.
- It upserts title, language, and catalogue identity through the owner-scoped
  Book alias and duplicate rules in ADR 0035.
- A new match becomes an active metadata-only My Books entry. No placeholder
  source, empty content record, scope, analysis, or deck is created.
- Repeated runs are idempotent. Metadata changes propagate without duplicating
  Books or membership.
- Entries removed upstream remain in My Books. Deleting a connection also
  leaves all Books, membership, acquired content, and derived history intact.

### Lazy content acquisition and per-book refresh

- Sync never downloads or re-downloads EPUB content.
- When the learner opens or chooses a metadata-only book for analysis, Mouseion
  begins a separate lazy content-acquisition flow. The precise trigger and
  inline-versus-River execution choice remain open for implementation design.
- Acquired state is published only after complete EPUB download, validation,
  and immutable snapshot persistence, as required by ADR 0035.
- Each catalogue-backed Book offers a metadata refresh for that entry. Refresh
  is an idempotent metadata-only upsert and has no scope or analysis effect.
- If the individual upstream entry is no longer present, refresh is a calm
  no-op that preserves the Book and its current metadata.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No connections and empty My Books | Explain why a connection is needed and that sync records metadata before content. | Add catalogue connection |
| Never synced | Identify the connection and explain which ready study languages, excluding English, are eligible. | Sync now |
| Syncing | Preserve existing collection and show that metadata reconciliation is operational work. | View operational status |
| Last synced | Show the last successful time and retain ordinary browse/edit/delete actions. | Sync now or browse |
| Sync failed | Name the connection, preserve prior data, and show an actionable reason. | Edit connection or retry |
| Metadata-only Book | Identify that content is not yet acquired and that analysis is unavailable until it is. | Open the book and express acquisition intent |
| Lazy acquisition running or failed | Preserve book context and distinguish content work from analysis. | View status or retry |
| Individual metadata refresh complete | Show refreshed metadata/last-refreshed state without implying content changed. | Return to book |

## Non-goals

- Downloading all content during sync or re-downloading content on resync.
- Detecting changed EPUB bytes or invalidating reviewed scope or analysis from
  sync.
- Destructively reconciling upstream removals.
- Synchronizing English, unready languages, or every non-English language.
- Adding a fourth destination or a global sync dashboard.
- Batch-selecting Books for analysis; whether a future batch contract exists is
  an open product question.
- Reintroducing an administrator role or administrator-managed catalogues.
- Resolving the default cadence, per-connection cadence configuration,
  last-synced storage shape, or exact lazy-acquisition trigger in this document.

## Acceptance criteria

- A fresh learner with no connection sees a clear My Books explanation and a
  primary action to `/connections` through Add books.
- A connection can be never synced, syncing, last synced, or failed, and every
  state has a usable server-rendered path and recovery where applicable.
- **Sync now** and periodic execution invoke the same owner-scoped,
  idempotent reconciliation.
- Only ready saved study languages except English are walked.
- Repeated runs create no duplicates; upstream metadata changes update local
  metadata; upstream removal and connection deletion remove nothing from My
  Books.
- Sync performs no EPUB download and causes no scope or analysis invalidation.
- Per-book refresh updates metadata only and safely no-ops when the entry is
  missing.
- Operational jobs remain visible and recoverable through `/jobs` without
  changing primary navigation.
