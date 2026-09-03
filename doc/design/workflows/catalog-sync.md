# Catalogue sync to My Books workflow

Status: **Canonical learner-facing workflow.** It is the automated,
metadata-first path to the local My Books collection. Product behavior is proposed
by [ADR 0041](../../adr/0041-catalog-sync-metadata-first.md) and the
[Catalogue Sync feature](../../features/catalog-sync.md).

## Goal

Help a learner connect the Calibre-Web catalogue they already curate, reconcile
its studyable bibliographic metadata into My Books, find a Book locally, and
acquire content only when they express intent to use that Book.

## Learner questions

The workflow answers these questions in order:

1. Where should Mouseion look for my collection?
2. Has this connection ever synchronized, and what happened most recently?
3. Which Books were added or updated for languages Mouseion and I are ready to
   study?
4. How can I find one Book in my local collection?
5. Does this Book have content, or must Mouseion acquire it first?
6. What scope do I want to analyze?

## Primary path

```text
My Books empty state
    -> Add learner-owned catalogue connection
    -> Sync now (then periodic reconciliation)
    -> Browse/search the local My Books collection
    -> Open a metadata-only Book
    -> Express intent and lazily acquire validated EPUB content
    -> Review scope (unchanged)
    -> Explicitly start analysis (unchanged)
```

### 1. Connect

The fresh My Books state explains that a catalogue connection lets Mouseion add
ready-language bibliographic entries before content is needed. Its primary
action enters `/connections` through the existing **Add books** workflow.

The connection remains learner-owned. The form identifies its name, URL,
username, and optional password; the saved secret is encrypted at rest and is
not displayed on edit. Deleting the connection removes future access and sync
configuration only. It never deletes My Books membership, content, analyses,
decks, or vocabulary.

### 2. Synchronize metadata

**Learner question:** Is Mouseion up to date with this catalogue?

The connection surface shows one of never synced, syncing, last synced, or
failed, and reports the eligible ready study-language scope. If no study
language is eligible, it directs the learner to Settings rather than claiming
the collection is current. **Sync now** submits the same owner-scoped River job used by the
periodic schedule. The run examines only the learner's ready study languages,
excluding English, and upserts metadata-only My Books entries. It never
downloads EPUB content and never removes local state.

The connection remains usable while sync work runs. Detailed attempts and recovery live on `/jobs`; the connection surface retains
the concise learner-relevant status.

### 3. Browse and search My Books

**Learner question:** Which local Book do I want to inspect?

After reconciliation the learner returns to My Books. Language pills, including
an unknown-language bucket, global local-collection search, and paging support a
large collection. This is the
[My Books Collection Browsing](../../features/collection-browsing.md) contract,
not OPDS search. Title and author lead; edition/year and language follow;
evidence state remains supporting information.

### 4. Open a metadata-only Book and acquire content lazily

**Learner question:** What evidence exists for this Book, and what is required
before analysis?

Opening `/books/{id}` preserves the Book identity and states that the entry has
metadata only. The Book detail page offers explicit acquisition via
`POST /opds/acquire`. Mouseion acquires and validates the EPUB through its
recorded owner-scoped catalogue identity. The transition is explicit and
recoverable.

The Book becomes acquired only after a complete validated immutable snapshot is
persisted. A missing upstream entry or acquisition failure leaves the Book and
metadata intact and gives an actionable recovery path.

### 5. Review scope and analyze

After lazy acquisition succeeds, the shipped manual lifecycle is unchanged:
the learner reviews and confirms an immutable scope, then explicitly starts
analysis. Sync never confirms scope, enqueues analysis, prepares a deck, or
invalidates existing evidence.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No connections | Explain why a learner-owned connection is needed and that sync is metadata-first. | Add catalogue connection |
| Never synced | Name the connection and eligible ready-language scope, excluding English. | Sync now |
| Syncing | Preserve existing Books, identify metadata reconciliation as in progress, and provide operational detail without turning jobs into navigation. | View job status |
| Synced with changes upserted | Show the last-synced time and a factual added/updated summary without implying content was downloaded. | Browse My Books |
| Synced with no changes | Show the last-synced time and eligible languages; state that no eligible EPUB entries were found, or guide the learner to Settings when scope is empty. | Browse My Books |
| Sync failed | Name the affected connection, preserve prior state, and distinguish authentication/configuration failure from a retryable upstream failure. | Edit connection or retry |
| Metadata-only Book | Explain that bibliographic identity is present but EPUB content is not. | Express acquisition intent |
| Lazy acquisition running | Keep Book context and identify content acquisition separately from analysis. | View status when queued |
| Lazy acquisition failed or entry missing | Preserve the Book and metadata and state what could not be acquired. | Edit connection, retry, or return to My Books |
| Content acquired | State that scope review is now available and no analysis has started. | Review scope |

## Navigation rules

- The authenticated shell remains exactly My Books, Reading Journey, and Settings
  with no acquisition action in the top navigation; `/connections` (Add books)
  is reached from My Books, while My Books is the sole browse surface.
- `/connections` owns configuration and concise sync status. `/jobs` owns
  attempts, progress, cancellation, retry, and detailed failures. Neither is a
  new destination.
- My Books owns local language grouping, search, paging, and browsing of synced
  metadata. Book detail owns per-book EPUB acquisition.
- Book detail owns lazy acquisition context, scope review, analysis, and deck
  actions. The language-level aggregate lens remains evidence-only.
- HTMX may enhance forms and status regions, but connect, sync submission,
  collection browsing, Book opening, acquisition recovery, scope review, and
  analysis must retain coherent server-rendered paths.
- Dynamic updates use scoped live regions and preserve focus. Connection names,
  status text, Book identity, and recovery actions never rely on color alone.
