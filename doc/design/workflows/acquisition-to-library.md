# Acquisition to My Books workflow

Status: **Canonical shipped learner-facing workflow.** Catalogue sync creates
metadata-only My Books Books. Adding a Book to Reading Journey expresses reading
intent and acquires and analyzes the current EPUB with ensure-once semantics.
The historical **Add to library** label may remain in compatibility artifacts.

The [catalogue sync workflow](catalog-sync.md) creates metadata-first My Books
entries. This document defines the per-book content-acquisition step from My
Books through Reading Journey; sync never downloads content.

## Goal

Help a learner move a metadata-only My Books Book to trustworthy current
evidence by expressing reading intent through Reading Journey membership.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../features/explicit-scoped-analysis-workflow.md)
- [Language Support](../../features/language-support.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [ADR 0035: My Books membership and source provenance](../../adr/0035-my-books-membership-and-source-provenance.md)
- [ADR 0049: Reading intent triggers analysis](../../adr/0049-reading-intent-triggers-analysis.md)
- [ADR 0054: Retire the standalone analysis action](../../adr/0054-retire-standalone-analysis-action.md)

## Entry and destination decision

**Add books** names catalogue setup and sync maintenance on `/connections`. It
is not a peer destination beside My Books, Reading Journey, and Vocabulary and
does not appear in the top navigation. My Books is the sole browse surface.

The action enters connection maintenance at `/connections`:

- with no connections, the primary task is to add one;
- with connections, the primary task is to sync eligible metadata into My Books;
- connection creation and editing remain available without becoming the visual
  focus of ordinary acquisition.

When several connections exist, sync and per-book acquisition use explicit
owner-scoped connection identity without changing ownership or credentials.

## Primary path

```text
My Books empty state
    -> Add or choose catalogue connection
    -> Sync catalogue metadata for offered ready languages
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
Metadata-only Books remain fully operable from their My Books rows: **Refresh
metadata**, **Add to Reading Journey**, and removal. **Add to Reading Journey**
expresses reading intent and triggers acquisition plus ensure-once analysis. No
metadata-only detail page or upstream browser is exposed.

### 3. Express reading intent

**Learner question:** Am I ready to add this Book to my provisional Journey?

**Add to Reading Journey** first records reversible membership, then performs the
same acquisition and ensure-once analysis. Re-adding a removed Book reuses
current completed or in-flight work; reordering has no analysis side effect.

When acquisition cannot currently resolve, the Journey membership remains. The
Book stays in its learner-chosen position with unavailable/incomparable evidence
and an actionable recovery path; it is not treated as a failed membership.

Changed EPUB content creates a new current revision, but does not trigger a
background watcher. The learner expresses intent again to refresh evidence.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No catalogue connections | Explain why a connection is needed. | Add catalogue connection |
| Connections available | Show recognizable connection names, sync status, and maintenance separately. | Sync catalogue |
| No ready catalogue language | Explain that no offered non-English language currently has a ready NLP pipeline. | Retry sync or add a different catalogue |
| Catalogue/authentication failure | Name the affected connection and give a recovery path. | Edit connection or retry |
| Acquiring or analyzing | Disable duplicate submission and announce durable acquisition/analysis progress. | View status |
| Analysis complete | Show current evidence and the optional deck action. | Inspect analysis or prepare deck |
| Stale current content | Explain that existing evidence is for an older revision. | Re-analyze from the Journey card |
| Unavailable acquisition | Preserve the Book and any Journey membership; mark evidence unavailable/incomparable. | Retry or check catalogue connection |

## Navigation and responsive rules

- Destination navigation is exactly My Books, Reading Journey, and Vocabulary;
  Add books does not appear in the top navigation (reached via the `/connections`
  workflow from My Books and direct routes).
- The current acquisition context is the selected Book and owner-scoped
  connection, not an upstream feed path.
- On narrow screens, row metadata precedes the acquisition and refresh actions,
  which remain reachable without horizontal page scrolling.
- HTMX enhances real links and forms. A failed or unavailable enhancement must
  not turn the Book detail page into an unusable raw fragment response.
- Dynamic acquisition updates use scoped live regions and do not move focus on
  every update.
