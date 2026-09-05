# Acquisition to My Books workflow

Status: **Canonical shipped learner-facing workflow.** The acquisition control
says **Add to My Books** and creates or restores owner-scoped My Books membership
after the validated EPUB snapshot is persisted. My Books can also contain
metadata-only Books outside this acquisition path. The historical **Add to
library** label may remain in compatibility artifacts.

The [catalogue sync workflow](catalog-sync.md) creates metadata-first My Books
entries. This document defines the per-book content-acquisition step from Book
detail; sync never downloads content.

## Goal

Help a learner acquire and validate one EPUB for a metadata-only My Books Book
without starting analysis.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../features/explicit-scoped-analysis-workflow.md)
- [Language Support](../../features/language-support.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [ADR 0035: My Books membership and source provenance](../../adr/0035-my-books-membership-and-source-provenance.md)

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
    -> Open metadata-only Book
    -> Acquire EPUB content
    -> Review scope and analyze explicitly
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

### 2. Browse My Books and acquire one Book

**Learner question:** Which books can Mouseion analyze with the currently ready
NLP service?

My Books owns language pills, local search, and paging. Opening a metadata-only
Book shows its bibliographic identity and an explicit **Acquire EPUB content**
form, submitted to `POST /opds/acquire`. The signed target carries the
owner-scoped connection and a fresh
catalogue download link; no upstream browser is exposed.

### 3. Acquire EPUB content

**Learner question:** Was this EPUB safely added, and can I continue browsing?

The Book detail action downloads and validates
the EPUB, resolves the owner-scoped Book and duplicate rules from ADR 0035,
stores immutable source content and extracted-unit identity, creates or restores
My Books membership, and does not confirm a scope or start analysis. A Book that
was already in My Books as metadata-only becomes acquired only after the full
validated snapshot is stored; no partial source state is published.

Success returns to the Book detail page to review scope. Repeated
acquisition resolves by owner-scoped Book aliases and immutable content digest.
It reports an existing acquired revision without error, or creates a new
immutable revision when the same source identifier now supplies different
bytes. Ambiguous identity conflicts are actionable and never silently merge
Books or replace source evidence.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No catalogue connections | Explain why a connection is needed. | Add catalogue connection |
| Connections available | Show recognizable connection names, sync status, and maintenance separately. | Sync catalogue |
| No ready catalogue language | Explain that no offered non-English language currently has a ready NLP pipeline. | Retry sync or add a different catalogue |
| Catalogue/authentication failure | Name the affected connection and give a recovery path. | Edit connection or retry |
| Acquiring EPUB | Disable duplicate submission and announce progress. | None |
| Acquired | Show the acquired Book and make scope review available. | Review scope |
| Already acquired | Link to the existing Book without treating idempotency as failure. | Open book |
| Invalid/non-EPUB acquisition | Explain that no usable content was added. | Return to Book |

## Navigation and responsive rules

- Destination navigation is exactly My Books, Reading Journey, and Vocabulary;
  Add books does not appear in the top navigation (reached via the `/connections`
  workflow from My Books and direct routes).
- The current acquisition context is the selected Book and owner-scoped
  connection, not an upstream feed path.
- On narrow screens, entry metadata precedes the acquisition action and actions
  remain reachable without horizontal page scrolling.
- HTMX enhances real links and forms. A failed or unavailable enhancement must
  not turn the Book detail page into an unusable raw fragment response.
- Dynamic acquisition updates use scoped live regions and do not move focus on
  every update.
