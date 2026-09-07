# Screen inventory

Status: **Canonical shipped learner-facing screen inventory.** It includes the
one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent analysis trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md). Catalogue-sync,
collection-browsing, and language-lens surfaces are owned by
[ADR 0041](../adr/0041-catalog-sync-metadata-first.md) and
[ADR 0042](../adr/0042-derived-language-corpus-view.md) and are marked planned
below. It is not a wireframe,
implementation plan, or persistence contract. Feature documents and ADRs
continue to own product behavior and historical decision details.

Every screen accounts for applicable loading, empty, error, disabled, success,
historical, degraded, stale-evidence, and narrow-viewport states. Mutation and
status endpoints that only support a parent screen are listed with that screen
rather than as independent destinations.

## Global shell

Authenticated screens use one shared shell with **Mouseion**, **My Books**,
**Reading Journey**, **Vocabulary**, account identity, and **Log
out**. My Books, Reading Journey, and Vocabulary are the only destinations; there
is no acquisition action in the top navigation. Catalogue setup and sync
maintenance are reached from My Books empty states and actions and via
`/connections`. My Books is the sole browse surface; Book detail owns explicit
analysis and acquires the EPUB when needed.

Primary Goal is embedded in Reading Journey and is not a fourth destination.
The shell identifies the current destination, supports skip navigation and
keyboard use, and preserves a clear path back to the parent book or Journey.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shipped application routes `/` to `/library`, serves Reading Journey at
`/journey`, and redirects `GET /campaigns` to `/journey`. Campaign history and
operations remain a secondary section on the Journey page; no parallel learner-
facing queue or plan is exposed.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Shipped `GET /library`; catalogue onboarding and sync are reached through `/connections` | Find a Book by bibliographic identity and understand its Goal/Journey relationship and trustworthy evidence state. | Book detail, Add books, or Add to Reading Journey | Empty collection with no connections and primary `/connections` action, metadata-only Book, search/filter empty, Primary Goal, in Journey, outside Journey, unassessed, stale/questionable evidence, cannot currently assess, ready to analyze, analysis queued/running/failed/complete, reading finished, long/missing metadata |
| My Books collection browser | Planned (Proposed) within `GET /library` | Find a Book in the local collection by language or text and move through a large result set. | Book detail or clear/revise controls | Language pills, unknown/no-language bucket, global local search, paging, combined filters, no match, later page removed, long content, enhancement unavailable |
| Per-language lens panel | Planned (Proposed) within `GET /library` language view | Understand analyzed count, aggregate current known coverage, highest-impact unknown vocabulary, and per-book spread for one language. | `/books/{id}` zoom | No analyzed Books, current evidence, mixed included/excluded Books, stale/incomplete evidence, known-vocabulary change, long lists; evidence-only with no lifecycle action and no learner-facing **Corpus** label |
| Book detail and analysis insights | Current `GET /books/{id}`; metadata refresh is a supporting action | Understand one Book, its lifecycle, current analysis evidence, Journey/Goal relationship, and available decisions on the sole learner-facing analysis surface. | Start analysis, analysis status, deck preparation, Journey/Goal action, prepared artifact, or metadata refresh | Source unavailable, metadata-only target state, lazy acquisition/analysis recovery, ready to analyze, analysis pending/failed/completed, stale or no current analysis, legacy/full-text state, warning-only analysis-quality note, prepared-deck state, reading/vocabulary facts, metadata refreshed, upstream entry missing/no-op |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Book detail when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or book detail | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Target `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the book's single current analysis surface. | Book detail | Valid owned book/run redirect, historical deep link, missing or unauthorized reference |
| Deck preparation | Target action on book detail; current preparation mutation/status/download routes remain | Consent to optional translation, prepare an APKG from the current analysis, recover failure, and download the ready artifact. | Download deck or return to Goal/book context | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalogue.
Title, author, and relevant edition information lead. Journey/Goal relationship
and evidence state follow. Large cards and metric-first sorting are not the
default.

The My Books model includes metadata-only and currently unassessable works.
Catalogue sync creates metadata-only membership; Start analysis and Reading
Journey intent acquire and validate EPUB content when needed.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Book detail | Show Book-centered lifecycle and relationship state plus the one current analysis: headline **Current known coverage** with a one-line analyzed-units qualifier, **Vocabulary investment**, **Highest-impact unknown vocabulary**, warning-only quality note, and deck preparation. It does not show analyzed-scope detail, text profile, projected token coverage, the broader coverage-stat list, or learner-facing analysis history. | Start analysis, analysis status, deck preparation, or Journey/Goal decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining book context. | **View analysis result** opens book detail when complete |
| Exact-analysis compatibility route | Redirect a valid historical result URL to book detail; do not render a distinct insight, identity, provenance, or history surface. | Book detail |
| Deck preparation | Provide a coherent server-rendered status baseline before enhancement. | Download deck or return to book/Goal context |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock current
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination. Historical classifier and
recommendation metadata is not a current learner-facing surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalogue connections | Current `GET/POST /connections` and mutation routes; sync states and **Sync now** | Add, edit, remove, or synchronize an owner-scoped OPDS connection. | Sync now, My Books, or operational job status | No connections, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation/authentication failure, deletion confirmation/error |
| Book-detail analysis/acquisition | Current `POST /books/{id}/analyze`, launched from Book detail | Explicitly acquire and analyze one My Books Book, or refresh stale evidence. | Analysis status or Book detail | Adding/analyzing, success, duplicate/idempotent existing run, unsupported/non-EPUB entry, download/validation failure |
| Per-book catalogue metadata refresh | Planned (Proposed) action on `GET /books/{id}` with a supporting mutation/status endpoint | Refresh one catalogue-backed Book's metadata without downloading content or changing evidence. | Book detail | Refreshing, updated, unchanged, upstream entry missing/no-op, connection failure; no scope or analysis invalidation |

The canonical Book-detail action is **Start analysis**: it acquires content when
needed and analyzes the complete current EPUB without adding Journey membership
or selecting a Primary Goal. **Add to Reading Journey** is the separate
learner-intent action that performs the same acquisition and ensure-once analysis.
Catalogue sync never performs either action.

## Reading Journey and Primary Goal

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Reading Journey | Shipped `GET /journey` (compatibility `GET /campaigns` redirects to `/journey`); Campaign history and operations are secondary on the page | Express reading intent, understand the current Primary Goal, freely shape a provisional order, and inspect current or conditional preparation evidence. | Primary Goal/book context, route comparison, My Books, or Where next? | Empty Journey, no Primary Goal, queued/running/current analysis, unassessed/incomparable book, acquisition unavailable, recalculating, recalculation failure, stale evidence, long content, narrow viewport |
| Route comparison and reorder preview | Embedded in Reading Journey | Compare **Your order** with an optional **Vocabulary-efficient alternative**, keep or adopt either, or make a manual change. | Updated Reading Journey | No comparable evidence, partial comparison, alternative available, manual preview, adopted change, neutral recalculation, failed recalculation |
| Primary Goal outcome / Where next? | Embedded shipped transitional state in Reading Journey | Understand what finishing the book actually changed and choose whether or where to commit next. | Choose as Primary Goal, reorder, My Books, continue vocabulary work, or no new Goal | Reading finished plus justified vocabulary transition, reading finished while vocabulary work remains, changed books, unchanged current evidence, no remaining Journey book, no next choice |
| Reading/preparation/vocabulary history | Secondary **Campaign history & operations** section on Reading Journey | Review factual past reading, preparation, completion, abandonment, and vocabulary-transition events without restoring Campaign as principal IA. | Book or Journey context | Empty history, mixed historical states, legacy Campaign terminology, unavailable artifact |

The Reading Journey is an ordered semantic list. The Primary Goal is anchored
above the provisional books. The learner's order remains canonical. A
vocabulary-efficient alternative contains only learner-selected books and is
clearly conditional evidence, never a recommendation about what to read.

Visible **Move earlier** and **Move later** controls are required. Drag may
enhance them. Reordering retains focus, announces the new position, and reports
recalculation in a scoped live region. On narrow screens, alternatives stack
without changing order or hiding authors and evidence labels.

### Goal outcome semantics

When reading is finished:

1. acknowledge **Reading finished** and end the book's current Primary Goal role;
2. state the justified vocabulary transition or that vocabulary work remains;
3. recalculate later books only from actual known vocabulary;
4. show precise changed or unchanged evidence;
5. ask **Where next?** without creating another Goal.

Do not say the Journey is complete, automatically choose another Goal, call a
book optimal, or claim vocabulary gains when the transition has not occurred.
The single-active Campaign remains internal reservation state and secondary
history/operations. ADR 0036 governs the independent reading outcome and
justified vocabulary transition shown by this surface.

## Derived study languages and known vocabulary

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Vocabulary | `GET /vocabulary`, `POST /vocabulary/import`, and import status endpoint | Select a language derived from My Books, upload one lemma per line, and understand imported, duplicate, and rejected rows. | Updated known-vocabulary list | No derived study languages, no selected language, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled |
| Direct known-vocabulary page | Compatibility `GET /known-vocab` | Redirect to the Vocabulary destination. | Vocabulary | Redirect to `/vocabulary`; remaining states belong to Vocabulary |

Vocabulary is the canonical destination for known vocabulary. Its language
picker is limited to the distinct normalized language tags of active
chosen-language Books. Changing a Book's language state does not remove books,
analyses, prepared artifacts, internal Campaigns, or known vocabulary. The
`/settings` compatibility route redirects to My Books; it is not a learner-facing
screen.

## Inactive and supporting implementation

- `Dashboard` is an inactive template and not a canonical destination.
- Shipped navigation is **My Books** / **Reading Journey** / **Vocabulary** with no
  acquisition action in the top navigation; historical **My Library**, **Learning**, queue, and learner-facing Campaign labels remain only as compatibility fallbacks/redirects and are not the accepted target IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery and Stitch artifacts are evidence, not additional screen
  contracts.
