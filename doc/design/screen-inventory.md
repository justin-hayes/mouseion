# Screen inventory

Status: **Canonical learner-facing screen inventory.** It includes the target
one-current-analysis contract proposed in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md), which remains
unshipped until its implementation issues land. Proposed catalogue-sync,
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
`/connections`. My Books is the sole browse surface; Book detail owns per-book
EPUB acquisition.

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
| My Books | Shipped `GET /library`; planned (Proposed) empty-state catalogue onboarding | Find a book by bibliographic identity and understand its Goal/Journey relationship and trustworthy evidence state. | Book detail, Add books, Add to Reading Journey, or Choose as Primary Goal | Empty collection with no connections and primary `/connections` action, metadata-only book, acquisition success, search/filter empty, Primary Goal, in Journey, outside Journey, unassessed, stale/questionable evidence, cannot currently assess, scope required, ready to analyze, analysis queued/running/failed/complete, reading finished, long/missing metadata |
| My Books collection browser | Planned (Proposed) within `GET /library` | Find a Book in the local collection by language or text and move through a large result set. | Book detail or clear/revise controls | Language pills, unknown/no-language bucket, global local search, paging, combined filters, no match, later page removed, long content, enhancement unavailable |
| Per-language lens panel | Planned (Proposed) within `GET /library` language view | Understand analyzed count, aggregate current known coverage, highest-impact unknown vocabulary, and per-book spread for one language. | `/books/{id}` zoom | No analyzed Books, current evidence, mixed included/excluded Books, stale/incomplete evidence, known-vocabulary change, long lists; evidence-only with no lifecycle action and no learner-facing **Corpus** label |
| Book detail and analysis insights | Target `GET /books/{id}`; planned (Proposed) metadata refresh | Understand one book, its lifecycle, current analysis evidence, Journey/Goal relationship, and available decisions on the sole learner-facing analysis surface. | Scope review, analysis status, deck preparation, Journey/Goal action, prepared artifact, or metadata refresh | Source unavailable, metadata-only target state, lazy content acquisition/recovery, no confirmed scope, scope confirmed, analysis pending/failed/completed, no current analysis, legacy/full-text state, warning-only analysis-quality note, prepared-deck state, reading/vocabulary facts, metadata refreshed, upstream entry missing/no-op |
| Scope review | Current `GET/POST /books/{id}/scope` | Review a calm native checklist of readable EPUB scope choices and confirm an immutable ordered scope. | Book detail | Reliable top-level TOC checklist or flat readable-unit fallback, all choices initially checked, Check all/Uncheck all, stale snapshot, empty selection, validation error preserving selection, successful confirmation |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Book detail when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or book detail | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Target `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the book's single current analysis surface. | Book detail | Valid owned book/run redirect, historical deep link, missing or unauthorized reference |
| Deck preparation | Target action on book detail; current preparation mutation/status/download routes remain | Consent to optional translation, prepare an APKG from the current analysis, recover failure, and download the ready artifact. | Download deck or return to Goal/book context | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalogue.
Title, author, and relevant edition information lead. Journey/Goal relationship
and evidence state follow. Large cards and metric-first sorting are not the
default.

The My Books model includes metadata-only and currently unassessable works.
Acquisition adds or restores owner-scoped My Books membership after the EPUB is
validated and its immutable source snapshot is persisted.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Book detail | Show book-centered lifecycle and relationship state plus the one current analysis: headline **Current known coverage** with a one-line analyzed-units qualifier, **Vocabulary investment**, **Highest-impact unknown vocabulary**, warning-only quality note, and deck preparation. It does not show analyzed-scope detail, text profile, projected token coverage, the broader coverage-stat list, or learner-facing analysis history. | Scope review, analysis status, deck preparation, or Journey/Goal decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining book context. | **View analysis result** opens book detail when complete |
| Exact-analysis compatibility route | Redirect a valid historical result URL to book detail; do not render a distinct insight, identity, provenance, or history surface. | Book detail |
| Deck preparation | Provide a coherent server-rendered status baseline before enhancement. | Download deck or return to book/Goal context |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock scoped
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination. Historical classifier
and recommendation metadata is not a current scope-review surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalogue connections | Current `GET/POST /connections` and mutation routes; sync states and **Sync now** | Add, edit, remove, or synchronize an owner-scoped OPDS connection. | Sync now, My Books, or operational job status | No connections, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation/authentication failure, deletion confirmation/error |
| Per-book EPUB acquisition | Current `POST /opds/acquire`, launched from Book detail | Acquire content for one metadata-only My Books Book. | Book detail or My Books | Adding, success, duplicate/idempotent existing book, unsupported/non-EPUB entry, download/validation failure |
| Per-book catalogue metadata refresh | Planned (Proposed) action on `GET /books/{id}` with a supporting mutation/status endpoint | Refresh one catalogue-backed Book's metadata without downloading content or changing evidence. | Book detail | Refreshing, updated, unchanged, upstream entry missing/no-op, connection failure; no scope or analysis invalidation |

The canonical design label for per-book content acquisition is **Acquire EPUB
content** and must not imply analysis,
Journey membership, or Primary Goal selection. The shipped implementation uses **Add to My Books** for My Books membership (compatibility copy **Add to library** may still appear in older compatibility strings/tests); broader collection semantics for metadata-only books shipped under [ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md).

## Reading Journey and Primary Goal

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Reading Journey | Shipped `GET /journey` (compatibility `GET /campaigns` redirects to `/journey`); Campaign history and operations are secondary on the page | Understand the current Primary Goal, freely shape a provisional order, and inspect current or conditional preparation evidence. | Primary Goal/book context, route comparison, My Books, or Where next? | Empty Journey, no Primary Goal, Goal with/without evidence, reorderable later books, unassessed/incomparable book, recalculating, recalculation failure, stale evidence, long content, narrow viewport |
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

## Settings and known vocabulary

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Settings | Current `GET /settings` and `/settings/languages` mutations | Manage study-language preferences. | Remain in Settings | No profile, ready languages, saved language while capability discovery is degraded, no newly available languages, add/remove success or failure |
| Vocabulary | `GET /vocabulary`, `POST /vocabulary/import`, and import status endpoint | Upload one lemma per line and understand imported, duplicate, and rejected rows. | Updated known-vocabulary list | No study languages, no selected language, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled |
| Direct known-vocabulary page | Compatibility `GET /known-vocab` | Redirect to the Vocabulary destination. | Vocabulary | Redirect to `/vocabulary`; remaining states belong to Vocabulary |

Vocabulary is the canonical destination for known vocabulary. Under the current
contract, removing a study-language preference does not remove books, analyses,
prepared artifacts, internal Campaigns, or known vocabulary for that language.
Journey/Goal effects must not be claimed before those relationships receive a
product contract.

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
