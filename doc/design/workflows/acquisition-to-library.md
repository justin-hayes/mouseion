# Acquisition to My Books workflow

Status: **Canonical learner-facing design workflow.** The shipped acquisition
contract currently uses **Add to library** and stores acquired EPUBs. The target
label **Add to My Books** must ship only with planner/feature-contract
reconciliation for the broader My Books model; this document does not redefine
source ownership or persistence.

## Goal

Help a learner connect a catalog, find books in a language Mouseion can
currently analyze, and add several EPUBs to My Books without starting
analysis or losing browse context.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../features/explicit-scoped-analysis-workflow.md)
- [Language Support](../../features/language-support.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)

## Entry and destination decision

**Add books** is a global workflow action, not a fourth peer destination beside
My Books, Reading Journey, and Settings. It remains persistently available in
the authenticated shell and should be visually distinguishable from destination
navigation.

The action enters the acquisition hub at `/connections`:

- with no connections, the primary task is to add one;
- with connections, the primary task is to choose a catalog to browse;
- connection creation and editing remain available without becoming the visual
  focus of ordinary acquisition.

Mouseion does not silently choose a catalog when several exist. It may offer a
clear **Browse** action for each connection and may remember browse context
within the current session without changing ownership or credentials.

## Primary path

```text
Add books
    -> Acquisition hub
    -> Add or choose catalog connection
    -> Choose a ready analysis language
    -> Browse, search, or follow a collection
    -> Add to My Books
    -> Remain in catalog context
    -> Add another book or open the owned book
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

### 2. Choose language and browse

**Learner question:** Which books can Mouseion analyze with the currently ready
NLP service?

Only languages advertised as ready by the NLP service are offered. Saved study
preferences and catalog browse languages are related but not identical:
choosing a catalog language does not add a study-language preference, and a
saved preference does not make an unavailable pipeline ready.

The catalog experience preserves:

- the selected connection and language;
- breadcrumbs and collection path;
- search query;
- pagination or feed position;
- entries already added during this browsing session.

Upstream, authentication, empty-feed, and search-empty states must be distinct.
An upstream failure is not presented as an indefinitely loading feed.

### 3. Add to My Books

**Learner question:** Was this EPUB safely added, and can I continue browsing?

Each eligible entry uses **Add to My Books**. The action downloads and validates
the EPUB, stores immutable source content and extracted-unit identity, and does
not confirm a scope or start analysis.

Success updates the entry in place and keeps the learner in the current feed.
The post-success choices are:

1. continue adding books;
2. open the owned book to review its scope.

Opening the book is secondary to preserving the multi-add workflow. Repeated
acquisition resolves to the existing owned book and says so without presenting
an error.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No catalog connections | Explain why a connection is needed. | Add catalog connection |
| Connections available | Show recognizable connection names and maintenance separately. | Browse catalog |
| No ready languages | Explain that analysis capability comes from the NLP service and cannot be enabled by changing a learner preference. | Return later |
| Feed loading | Preserve the existing page and identify the region being updated. | None |
| Feed ready | Show path, search, entries, and pagination. | Add to My Books |
| Empty feed | Distinguish an empty collection from failure. | Go back or search |
| Search empty | Retain the query and selected catalog. | Revise search |
| Upstream/authentication failure | Name the affected connection and give a recovery path. | Edit connection or retry |
| Adding EPUB | Disable duplicate submission and announce progress. | None |
| Added | Replace the entry action with owned state and an optional book link. | Continue browsing |
| Already owned | Link to the existing book without treating idempotency as failure. | Continue browsing or open book |
| Invalid/non-EPUB acquisition | Explain that no usable book was added. | Return to feed |

## Navigation and responsive rules

- Destination navigation is My Books, Reading Journey, and Settings; Add books
  is styled and announced as an action.
- The current acquisition context is the connection, language, and feed path,
  not a generic page title.
- On narrow screens, entry metadata precedes the acquisition action and actions
  remain reachable without horizontal page scrolling.
- HTMX enhances real links and forms. A failed or unavailable enhancement must
  not turn the catalog into an unusable selector or raw fragment response.
- Dynamic feed and acquisition updates use scoped live regions and do not move
  focus on every update.
