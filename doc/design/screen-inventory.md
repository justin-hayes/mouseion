# Screen inventory

Status: **Canonical shipped learner-facing screen inventory.** It includes the
one-current-analysis contract in
[ADR 0040](../adr/0040-one-current-analysis-per-book.md) and the
reading-intent analysis trigger in
[ADR 0049](../adr/0049-reading-intent-triggers-analysis.md), with the
standalone learner action retired by [ADR 0054](../adr/0054-retire-standalone-analysis-action.md).
Language is the
app's organizing mode per
[ADR 0050](../adr/0050-active-study-language.md) and
[ADR 0051](../adr/0051-reading-journeys-and-goals-per-language.md): one active
study language scopes My Books, Reading Journey, and Vocabulary, and Journeys
and Goals are one per language. Catalog maintenance is a fourth principal
destination per
[ADR 0058](../adr/0058-catalog-maintenance-principal-destination.md).
Catalog-sync and collection-browsing surfaces
follow [ADR 0041](../adr/0041-catalog-sync-metadata-first.md) and the related feature
contracts below. The proposed language-lens panel is retired by
[ADR 0057](../adr/0057-retire-language-view-panel.md) and has no current screen
contract. It is not a wireframe,
implementation plan, or persistence contract. Feature documents and ADRs
continue to own product behavior and historical decision details.

Every screen accounts for applicable loading, empty, error, disabled, success,
historical, degraded, stale-evidence, and narrow-viewport states. Mutation and
status endpoints that only support a parent screen are listed with that screen
rather than as independent destinations.

## Global shell

Authenticated screens use one shared shell with **Mouseion**, **My Books**,
**Reading Journey**, **Vocabulary**, **Catalogs**, account identity, and **Log
out**. My Books, Reading Journey, Vocabulary, and Catalogs are the destinations;
there is no acquisition action in the top navigation. Catalogs is the persistent
home for connection and sync maintenance, also reached from My Books empty states
and actions; the legacy `/connections` route redirects there. My Books is the
sole browse surface; its rows own acquisition and analysis intent.

Primary Goal is embedded in Reading Journey and is not a fifth destination.
The shell identifies the current destination, supports skip navigation and
keyboard use, and preserves a clear path back to the parent book or Journey.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shell also carries a native **active study language** control alongside the
four destinations, on every authenticated screen. It lists the learner's study
languages plus any known-vocabulary-only language marked "no books", marks a
newly arrived study language "new", and is keyboard-accessible and
server-rendered before enhancement. Changing it navigates to the same screen in
the new language on language-scoped screens and updates the stored mode
elsewhere; it never auto-switches on navigation or sync.

The shipped application routes `/` to `/library` and serves Reading Journey at
`/journey`. The Journey entry at `/journey/{bookID}` owns the current Book
vocabulary-study state and its per-Book study history; no parallel learner-facing
queue, campaign, or plan is exposed.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Shipped `GET /library`; catalog onboarding and sync are reached through the Catalogs destination | Find a Book by bibliographic identity and understand its Goal/Journey relationship and trustworthy evidence state. | Journey entry, Catalogs, or Add to Reading Journey | Empty collection with no connections, metadata-only Book, search/filter empty, Primary Goal, in Journey, outside Journey, unassessed, stale/questionable evidence, cannot currently assess, analysis queued/running/failed/complete, reading finished, long/missing metadata |
| My Books collection browser | Planned (Proposed) within `GET /library` | Find a Book in the active language's collection by text and move through a large result set. | Journey entry or clear/revise controls | Scoped to the active study language (no "All languages"), needs-language strip for Books awaiting a language, scoped search, paging, combined filters, no match, later page removed, long content, enhancement unavailable |
| Journey entry and analysis insights | Current `GET /journey/{bookID}` only for a Journey member with a current completed analysis; metadata refresh is a My Books row action | Understand one analyzed Book's current analysis evidence, Journey/Goal relationship, and preparation decisions. Unassessed, stale, queued, running, failed, and cancelled Books have no detail page. | Analysis status, deck preparation, Journey/Goal action, prepared artifact, or My Books | Current completed analysis, legacy/full-text state, warning-only analysis-quality note, prepared-deck state, reading/vocabulary facts |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Journey entry when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or Journey entry | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Shipped `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the current result context. | Journey entry for members; 404 otherwise | Valid owned member/run redirect, non-member or incomplete result, missing or unauthorized reference |
| Deck preparation | Target action on Journey entry; current preparation mutation/status/download routes remain | Consent to optional translation, prepare an APKG from the current analysis, recover failure, and download the ready artifact. | Download deck or return to Journey context | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalog.
Title, author, and relevant edition information lead. Journey/Goal relationship
and evidence state follow. Large cards and metric-first sorting are not the
default.

The My Books model includes metadata-only and currently unassessable works.
Catalog sync creates metadata-only membership; adding a Book to Reading
Journey acquires and validates EPUB content when needed. Metadata-only rows retain
refresh, Journey, and removal actions without opening a detail page.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Journey entry | Show the one current completed analysis: headline **Current known coverage** with a one-line analyzed-units qualifier, **Vocabulary investment**, **Highest-impact unknown vocabulary**, warning-only quality note, and deck preparation. It does not show analyzed-scope detail, text profile, projected token coverage, the broader coverage-stat list, or learner-facing analysis history. | Analysis status, deck preparation, or Journey/Goal decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining Journey context. | **View analysis result** opens the Journey entry when complete |
| Exact-analysis compatibility route | Redirect a valid current member result URL to the Journey entry; do not render a distinct insight, identity, provenance, or history surface. | Journey entry |
| Deck preparation | Provide a coherent server-rendered status baseline before enhancement. | Download deck or return to Journey/Goal context |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock current
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination. Historical classifier and
recommendation metadata is not a current learner-facing surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalogs | Shipped `GET /catalogs` (legacy `/connections` redirects) with connection mutation routes; sync states and **Sync now** | Add, edit, remove, or synchronize an owner-scoped OPDS connection. | Sync now, My Books, or operational job status | No connections, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation/authentication failure, deletion confirmation/error |
| Book acquisition through reading intent | Current `POST /journey/books/{id}/add`, launched from My Books or deck status | Express intent, acquire one My Books Book when needed, and ensure current analysis once. | Journey or analysis status | Adding/analyzing, success, duplicate/idempotent existing run, unsupported/non-EPUB entry, download/validation failure |
| Per-book catalog metadata refresh | Current row action on `GET /library` with `POST /library/books/{id}/refresh` | Refresh one catalog-backed Book's metadata without downloading content or changing evidence. | My Books row | Refreshing, updated, unchanged, upstream entry missing/no-op, connection failure; no scope or analysis invalidation |

Adding a Book to Reading Journey is the sole learner-facing acquisition and
analysis trigger. It acquires content when needed and ensures analysis without
selecting a Primary Goal. Catalog sync remains metadata-only, and My Books row
metadata refresh never invalidates or re-triggers analysis.

## Reading Journey and Primary Goal

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Reading Journey | Shipped `GET /journey` | Express reading intent, understand the current Primary Goal, freely shape a provisional order, and inspect current or conditional preparation evidence — all for the active study language's Journey. | Primary Goal/book context, route comparison, My Books, or Where next? | Empty Journey, no Primary Goal, queued/running/current analysis, unassessed/incomparable book, acquisition unavailable, recalculating, recalculation failure, stale evidence, long content, narrow viewport |
| Route comparison and reorder preview | Embedded in Reading Journey | Compare **Your order** with an optional **Vocabulary-efficient alternative**, keep or adopt either, or make a manual change, within the active language's Journey. | Updated Reading Journey | No comparable evidence, partial comparison, alternative available, manual preview, adopted change, neutral recalculation, failed recalculation |
| Primary Goal outcome / Where next? | Embedded shipped transitional state in Reading Journey | Understand what finishing the book actually changed and choose whether or where to commit next, for the active language's Goal. | Choose as Primary Goal, reorder, My Books, continue vocabulary work, or no new Goal | Reading finished plus justified vocabulary transition, reading finished while vocabulary work remains, changed books, unchanged current evidence, no remaining Journey book, no next choice; one Goal per language, other languages' Goals unaffected |
| Book vocabulary-study history | Secondary section on the Journey entry at `/journey/{bookID}` | Review prior vocabulary studies for the Book while preserving each prepared deck's provenance. | Book or Journey context | Empty history, studying, reviewed, released, unavailable artifact |

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
justified vocabulary transition shown by this surface. Goals are one per study
language (ADR 0051): finishing a Goal in the active language does not touch
other languages' Goals.

## Active study language, derived study languages, and known vocabulary

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Vocabulary | `GET /vocabulary`, `POST /vocabulary/import`, and import status endpoint | View and import known vocabulary for the active study language: upload one lemma per line and understand imported, duplicate, and rejected rows. | Updated known-vocabulary list | No derived study languages, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled; scoped to the active language with no per-page picker |
| Direct known-vocabulary page | Compatibility `GET /known-vocab` | Redirect to the Vocabulary destination. | Vocabulary | Redirect to `/vocabulary`; remaining states belong to Vocabulary |

Vocabulary is the canonical destination for known vocabulary, scoped to the
active study language; its per-page language picker is removed in favour of the
shell-level switcher. Known-vocabulary-only languages (no current chosen-language
Book) remain selectable there as read-only "no books" entries; import stays
limited to the derived study-language set. Changing a Book's language state does
not remove books, analyses, prepared artifacts, vocabulary-study history, or
known vocabulary. The `/settings` compatibility route redirects to My Books; it is not a
learner-facing screen.

## Inactive and supporting implementation

- `Dashboard` is an inactive template and not a canonical destination.
- Shipped navigation is **My Books** / **Reading Journey** / **Vocabulary** /
  **Catalogs** with no acquisition action in the top navigation; historical
  **My Library**, **Learning**, queue, and learner-facing Campaign labels remain
  only as compatibility fallbacks/redirects and are not the accepted target IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery and Stitch artifacts are evidence, not additional screen
  contracts.
