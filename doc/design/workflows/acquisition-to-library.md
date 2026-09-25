# Acquisition to My Books workflow

Status: **Canonical shipped learner-facing workflow.** Catalog sync creates
metadata-only My Books Books. Adding a Book to Reading Journey expresses reading
intent and acquires and analyzes the current EPUB with ensure-once semantics.
The historical **Add to library** label may remain in compatibility artifacts.

The [catalog sync workflow](catalog-sync.md) creates metadata-first My Books
entries. This document defines the per-book content-acquisition step from My
Books through Reading Journey; sync never downloads EPUB content. Independent
optional Book cover retrieval follows
[ADR 0077](../../adr/0077-catalog-supplied-book-covers.md).

## Goal

Help a learner move a metadata-only My Books Book to trustworthy current
evidence by expressing reading intent through Reading Journey membership.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../archive/features/explicit-scoped-analysis-workflow.md)
- [Language Support](../../features/language-support.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [ADR 0035: My Books membership and source provenance](../../adr/0035-my-books-membership-and-source-provenance.md)
- [ADR 0049: Reading intent triggers analysis](../../adr/0049-reading-intent-triggers-analysis.md)
- [ADR 0054: Retire the standalone analysis action](../../adr/0054-retire-standalone-analysis-action.md)

## Entry and destination decision

**Catalogs** names catalogue setup and sync maintenance on `/catalogs`. It is a
peer destination beside My Books, Reading Journey, and Vocabulary. My Books is
the sole browse surface.

The action enters connection maintenance at `/catalogs`:

- with no connections, the primary task is to add one;
- with connections, the primary task is to sync eligible metadata into My Books;
- connection creation and editing remain available without becoming the visual
  focus of ordinary acquisition.

When several connections exist, sync and per-book acquisition use explicit
owner-scoped connection identity without changing ownership or credentials.

## Primary path

```text
My Books empty state
    -> Add or choose catalog connection
    -> Sync catalog metadata for offered ready languages
    -> Browse My Books locally
    -> Add Book to Reading Journey
    -> Acquire current EPUB and ensure whole-book analysis
    -> Inspect Book or Journey evidence
```

### 1. Configure a catalog connection

**Learner question:** Where should Mouseion look for my books?

The screen must distinguish the first-connection task from maintenance of saved
connections. Credentials are learner-owned and encrypted at rest. Password
fields never display a saved secret; leaving an edit password blank preserves
the current credential.

The interface must answer:

- Is this connection saved and usable?
- Which URL and learner-visible name identify it?
- Does editing affect already acquired books? It does not.
- What will deleting the connection do? It removes future access through that
  connection; it does not delete owned books or their artifacts.

Connection deletion is consequential and requires confirmation that states this
boundary.

### 2. Browse My Books and choose a Book

**Learner question:** Which books can Mouseion analyze with the currently ready
NLP service?

My Books owns the active-language-scoped browse, search, and paging; there is no
"All languages" default ([ADR 0050](../../adr/0050-active-study-language.md)).
The **All**, **Inbox**, **To Read**, and **Set Aside** filters organize Books by
disposition, with reading completion retained as independent history. A learner
can move an Inbox or Set Aside Book to **To Read**, expressing reading intent and
triggering acquisition plus ensure-once analysis, or set aside a non-current
Book while retaining its content, analysis, provenance, and history. Metadata-only
Books remain operable from their My Books items. No metadata-only detail page or
upstream browser is exposed.

### 3. Express reading intent

**Learner question:** Am I ready to add this Book to my provisional Journey?

**Add to Reading Journey** first records reversible membership, then performs the
same acquisition and ensure-once analysis. Re-adding a removed Book reuses
current completed or in-flight work; reordering has no analysis side effect.

When acquisition cannot currently resolve, the Journey membership remains. The
Book stays in its learner-chosen position with unavailable evidence and an
actionable recovery path; it is not treated as a failed membership.

Changed EPUB content creates a new current revision, but does not trigger a
background watcher. The learner expresses intent again to refresh evidence.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No catalog connections | Explain why a connection is needed. | Add catalog connection |
| Connections available | Show recognizable connection names, sync status, and maintenance separately. | Sync catalog |
| No ready catalog language | Explain that no offered non-English language currently has a ready NLP pipeline. | Retry sync or add a different catalog |
| Catalog/authentication failure | Name the affected connection and give a recovery path. | Edit connection or retry |
| Acquiring or analyzing | Disable duplicate submission and announce durable acquisition/analysis progress. | View status |
| Analysis complete | Show current evidence and the optional deck action. | Inspect analysis or prepare deck |
| Stale current content | Explain that existing evidence is for an older revision. | Re-analyze from the Journey card |
| Unavailable acquisition | Preserve the Book and any Journey membership; mark evidence unavailable. | Retry or check catalog connection |

## Navigation and responsive rules

- Destination navigation is exactly My Books, Reading Journey, Vocabulary, and
  Catalogs. The legacy `GET /connections` route permanently redirects to
  `/catalogs` while preserving supported deep-link parameters.
- The current acquisition context is the selected Book and owner-scoped
  connection, not an upstream feed path.
- On narrow screens, Book identity precedes the acquisition and refresh actions,
  which remain reachable without horizontal page scrolling.
- HTMX enhances real links and forms. A failed or unavailable enhancement must
  not turn the My Books item into an unusable raw fragment response.
- Dynamic acquisition updates use scoped live regions and do not move focus on
  every update.
