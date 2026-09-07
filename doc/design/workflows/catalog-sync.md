# Catalogue sync to My Books workflow

Status: **Canonical learner-facing workflow.** It is the automated,
metadata-first path to the local My Books collection. Product behavior is defined
by the metadata-first and non-destructive contract in
[ADR 0041](../../adr/0041-catalog-sync-metadata-first.md), with its language
scope reconciled by [ADR 0043](../../adr/0043-study-languages-derived-settings-removed.md),
and by the [Catalogue Sync feature](../../features/catalog-sync.md).
Reading Journey's analysis consequence is defined by [ADR
0049](../../adr/0049-reading-intent-triggers-analysis.md).

## Goal

Help a learner connect the Calibre-Web catalogue they already curate, reconcile
its studyable bibliographic metadata into My Books, find a Book locally, and
express reading intent only when they are ready for acquisition and analysis.

## Learner questions

The workflow answers these questions in order:

1. Where should Mouseion look for my collection?
2. Has this connection ever synchronized, and what happened most recently?
3. Which Books were added or updated for non-English languages whose NLP
   pipelines are ready?
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
    -> Start analysis explicitly, or add the Book to Reading Journey
    -> Lazily acquire validated EPUB content and ensure whole-book analysis
    -> Inspect Book or Journey evidence
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
failed, and reports that the run walks offered non-English languages whose NLP
pipelines are ready. **Sync now** submits the same owner-scoped River job used by
the periodic schedule. The run does not consult a saved study-language
selection: it upserts metadata-only My Books entries for every eligible
catalogue language, excluding English. It never downloads EPUB content and
never removes local state. The chosen-language Books then define the learner's
derived study-language set.

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
metadata only. The Book detail page offers **Start analysis**, which acquires,
validates, and analyzes the EPUB in one explicit action. It also offers **Add to
Reading Journey**, which records reversible reading intent and performs the same
acquisition plus ensure-once analysis. Both paths use the recorded owner-scoped
catalogue identity.

The Book becomes acquired only after a complete validated immutable snapshot is
persisted. A missing upstream entry or acquisition failure leaves the Book and
metadata intact and gives an actionable recovery path.

### 5. Analyze and recover

Sync itself remains metadata-only. **Start analysis** is the explicit manual
path for a My Books Book and the refresh lever after content changes. Adding a
Book to Reading Journey is the only automatic exception: it ensures current
whole-book analysis once, without a background watcher. Re-adding and reordering
do not duplicate work. Sync never acquires content, enqueues analysis, prepares a
deck, or invalidates existing evidence.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No connections | Explain why a learner-owned connection is needed and that sync is metadata-first. | Add catalogue connection |
| Never synced | Name the connection and explain the offered non-English ready-language scope. | Sync now |
| Syncing | Preserve existing Books, identify metadata reconciliation as in progress, and provide operational detail without turning jobs into navigation. | View job status |
| Synced with changes upserted | Show the last-synced time and a factual added/updated summary without implying content was downloaded. | Browse My Books |
| Synced with no changes | Show the last-synced time and state that no eligible EPUB entries were found when the reconciliation produced no changes. | Browse My Books |
| Sync failed | Name the affected connection, preserve prior state, and distinguish authentication/configuration failure from a retryable upstream failure. | Edit connection or retry |
| Metadata-only Book | Explain that bibliographic identity is present but EPUB content is not. | Start analysis or add to Reading Journey |
| Acquisition/analysis running | Keep Book or Journey context and identify durable progress. | View status |
| Acquisition failed or entry missing | Preserve the Book, metadata, and any Journey membership; state what could not be acquired. | Edit connection or retry |
| Current analysis complete | State that evidence is available for the current content revision. | Inspect analysis or prepare deck |
| Stale analysis | State that content changed and existing evidence is older. | Start analysis |

## Navigation rules

- The authenticated shell remains exactly My Books, Reading Journey, and
  Vocabulary with no acquisition action in the top navigation; `/connections`
  (Add books)
  is reached from My Books, while My Books is the sole browse surface.
- `/connections` owns configuration and concise sync status. `/jobs` owns
  attempts, progress, cancellation, retry, and detailed failures. Neither is a
  new destination.
- My Books owns local language grouping, search, paging, and browsing of synced
  metadata. Book detail owns per-book EPUB acquisition.
- Book detail owns lazy acquisition context, Start analysis, current analysis,
  and deck actions. Reading Journey owns reading intent and its ensure-once
  analysis consequence. The language-level aggregate lens remains evidence-only.
- HTMX may enhance forms and status regions, but connect, sync submission,
  collection browsing, Book opening, acquisition recovery, Start analysis, and
  analysis status must retain coherent server-rendered paths.
- Dynamic updates use scoped live regions and preserve focus. Connection names,
  status text, Book identity, and recovery actions never rely on color alone.
