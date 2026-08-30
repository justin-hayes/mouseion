# Screen inventory

This inventory records the current server-rendered frontend at the route and
learner-goal level. It is not a wireframe specification. Feature documents and
ADRs continue to own product behavior.

Every screen must account for applicable loading, empty, error, disabled,
success, historical, degraded, and narrow-viewport states. Mutation and status
endpoints that only support a parent screen are listed with that screen rather
than as independent destinations.

## Global shell

Authenticated screens use a shared shell with Mouseion, My Library, Learning,
Add books, Settings, the username, and Log out. My Library, Learning, and
Settings are destinations; Add books is a persistent workflow action. The shell
should identify the current destination, distinguish the acquisition action,
support keyboard navigation, and preserve a clear path back to the parent book
or campaign. `/` redirects to `/library`; there is no active dashboard screen.

## Authentication

| Screen | Route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | `GET /login`, `POST /onboarding` | Create the first local learner account. | My Library | Fresh installation, validation error, password requirements, submission failure |
| Sign in | `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Library | Invalid credentials, expired session redirect, service error |

## Library and book analysis

| Screen | Route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Library | `GET /library` | Find an owned book and understand what it needs next. | Book detail or Add books | Empty library, acquisition success, scope required, ready to analyze, queued/running/failed/completed analysis, deck/campaign state when available |
| Book detail and analysis insights | `GET /books/{id}` | Understand one book, its exact analysis provenance, readiness, and next action. | Scope review, start analysis, analysis status, deck/campaign action | Source unavailable, no confirmed scope, scope confirmed, analysis pending/failed/completed, insights unavailable, legacy/full-text result, quality warning, prepared-deck state |
| Scope review | `GET/POST /books/{id}/scope` | Review recommendations, choose EPUB units, compare/reuse history, and confirm an immutable scope. | Book detail | Recommended/all/prior preset, grouped or flat structure, partially selected group, uncertain units, stale snapshot, empty selection, validation error preserving selection, successful confirmation |
| Analysis status | `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Exact analysis result when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | `GET /jobs` | Inspect owner-scoped operational analysis history. | Individual analysis status | Empty history, mixed states, historical/legacy records |
| Deck preparation | Embedded in the exact analysis result; `POST /books/{book-id}/analyses/{analysis-run-id}/deck/preparations` and `/deck-preparations/{id}/*` | Consent to optional translation, prepare an APKG, recover failure, and download the ready artifact. | Download deck or add to learning queue | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

The accepted lifecycle places analysis insights between completion and deck
preparation. Completed scoped jobs now link from operational status to the
exact result; legacy jobs retain their operational status without scoped deck
preparation.

### Approved Phase 1 continuity

The approved implementation target separates operational status from the exact
completed result:

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Book detail | Show the learner-facing next state and analysis history. Active runs link to status; completed scoped runs link to exact results. | Scope review, analysis status, or exact analysis result |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining book context. | `View analysis result` when complete |
| Exact analysis result | At `/books/{book-id}/analyses/{analysis-run-id}`, show identity, quality, insights, provenance, and eligible deck preparation for one immutable result. | Prepare deck, return to book, or review scope |
| Deck preparation | Live on the exact analysis result with a coherent server-rendered status baseline before enhancement. | Download deck or add to learning queue |

The exact-result route is the shipped behavior for completed scoped analyses.
Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock scoped
deck preparation.

## Book acquisition

| Screen | Route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalog connections | `GET/POST /connections` and connection mutation routes | Add, edit, browse, or remove an owner-scoped OPDS connection. | Catalog browser | No connections, saved connection, credentialed connection, validation/authentication failure, deletion confirmation/error |
| Catalog browser | `GET /catalog` with `/opds/language`, `/opds/browse`, and `/opds/search` fragments | Find EPUBs in one ready analysis language without losing feed context. | Add to library or Book detail | No connection, no ready language, capability discovery degraded, feed loading, breadcrumbs, pagination, search results, empty feed/search, upstream error |
| Catalog entry acquisition | Embedded `POST /opds/acquire` result | Add one EPUB to the library and continue browsing. | Remain in feed or open existing/new book | Adding, success, duplicate/idempotent existing book, unsupported/non-EPUB entry, download/validation failure |

Acquisition copy must say **Add to library** and must not imply that analysis
starts automatically.

**Add books** is an authenticated-shell workflow action rather than a peer
destination. It enters `/connections`, where first-time setup or catalog choice
precedes browsing. The action should remain persistently available while being
visually distinguishable from My Library, Learning, and Settings.

## Learning

| Screen | Route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Learning campaigns | `GET /campaigns` and campaign mutation routes | Understand the active book/deck, manage the queue, record reading/review progress, and inspect history. | Active campaign, downloaded deck, or queued campaign | No active campaign, active reading/study combinations, prepared deck available, queued campaigns, start blocked by existing active campaign, complete, abandoned, action failure |

The action that satisfies the second completion condition is consequential: it
may graduate all campaign vocabulary into known vocabulary. The screen must
explain that outcome before the transition and must not call assignment or deck
review “mastery.”

When one progress condition is already satisfied, the remaining action opens a
completion confirmation that names the campaign, states how many assigned
lemmas will be newly added to known vocabulary when available, explains
coverage recalculation, and
discloses that completion cannot currently be undone in Mouseion. Abandonment
has its own confirmation explaining that artifacts remain while reserved
vocabulary becomes eligible again.

## Settings and known vocabulary

| Screen | Route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Settings | `GET /settings` and `/settings/languages` mutations | Manage study-language preferences and known vocabulary. | Remain in Settings | No profile, ready languages, saved language while capability discovery is degraded, no newly available languages, add/remove success or failure |
| Known-vocabulary import | Embedded in Settings; `POST /known-vocab/import` and import status endpoint | Upload one lemma per line and understand imported, duplicate, and rejected rows. | Updated known-vocabulary list | No selected language, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled |
| Direct known-vocabulary page | `GET /known-vocab` | Retained compatibility route that redirects to Settings. | Settings | Redirect to `/settings#known-vocabulary`; remaining states are the embedded Settings-known-vocabulary states |

Settings is the canonical primary-navigation entry. `/known-vocab` is a retained
secondary route that redirects to `/settings#known-vocabulary` while preserving
valid language context; it is not a competing IA. Removing a study-language
preference does not remove books or known vocabulary for that language.

## Inactive and supporting implementation

- `Dashboard` is an inactive template. It is not reachable from `/`, and its
  older `/languages` destination is not part of the current route map.
- Enrichment, deck-preparation, and known-vocabulary-import status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and
  should not be treated as complete pages in future design work.
