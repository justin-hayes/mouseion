# Screen inventory

Status: **Canonical target screen inventory with current-route compatibility
notes.** It is not a wireframe, implementation plan, or persistence contract.
Feature documents and ADRs continue to own shipped product behavior until the
planner/ADR boundaries in
[`information-architecture.md`](information-architecture.md#contract-changes-requiring-planneradr-work)
are resolved.

Every screen accounts for applicable loading, empty, error, disabled, success,
historical, degraded, stale-evidence, and narrow-viewport states. Mutation and
status endpoints that only support a parent screen are listed with that screen
rather than as independent destinations.

## Global shell

Authenticated screens use one shared shell with **Mouseion**, **My Books**,
**Reading Journey**, **Add books**, **Settings**, account identity, and **Log
out**. My Books, Reading Journey, and Settings are destinations. Add books is a
persistent workflow action styled and announced differently from navigation.

Primary Goal is embedded in Reading Journey and is not a fourth destination.
The shell identifies the current destination, supports skip navigation and
keyboard use, and preserves a clear path back to the parent book or Journey.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The current application still routes `/` to `/library` and exposes `/campaigns`.
My Books may continue to use `/library`; the future Reading Journey route is a
planner/implementation decision. Do not create parallel navigation or imply
that the current campaign queue already has Journey semantics.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Current `GET /library`; canonical label changes while broader membership awaits contract work | Find a book by bibliographic identity and understand its Goal/Journey relationship and trustworthy evidence state. | Book detail, Add books, Add to Reading Journey, or Choose as Primary Goal when supported | Empty collection, acquisition success, search/filter empty, Primary Goal, in Journey, outside Journey, unassessed, stale/questionable evidence, cannot currently assess, scope required, ready to analyze, analysis queued/running/failed/complete, reading finished, long/missing metadata |
| Book detail and analysis insights | Current `GET /books/{id}` | Understand one book, its exact identity and provenance, current evidence, Journey/Goal relationship, and available decisions. | Scope review, analysis workflow, Journey/Goal action, or prepared artifact | Source unavailable, metadata-only target state, no confirmed scope, scope confirmed, analysis pending/failed/completed, insights unavailable, legacy/full-text result, quality warning, prepared-deck state, reading/vocabulary facts |
| Scope review | Current `GET/POST /books/{id}/scope` | Review recommendations, choose EPUB units, compare/reuse history, and confirm an immutable scope. | Book detail | Recommended/all/prior preset, grouped or flat structure, partially selected group, uncertain units, stale snapshot, empty selection, validation error preserving selection, successful confirmation |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Exact analysis result when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history. | Individual analysis status or exact result | Empty history, mixed states, historical/legacy records |
| Exact analysis result | Current `GET /books/{book-id}/analyses/{analysis-run-id}` | Evaluate one immutable result's identity, quality, current coverage, conditional preparation evidence, and next action. | Prepare deck, return to book, or review scope | Trustworthy result, quality warning, stale vocabulary comparison, missing insights, legacy incompatibility, deck eligibility |
| Deck preparation | Embedded in exact result; current preparation mutation/status/download routes | Consent to optional translation, prepare an APKG, recover failure, and download the ready artifact. | Download deck or return to Goal/book context | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalogue.
Title, author, and relevant edition information lead. Journey/Goal relationship
and evidence state follow. Large cards and metric-first sorting are not the
default.

The broader My Books model includes metadata-only and currently unassessable
works. The current `/library` model owns acquired EPUBs only. That expansion is
a planner/product-contract requirement, not a template-only change.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Book detail | Show book-centered lifecycle and relationship state; active runs link to status and completed scoped runs link to exact results. | Scope review, analysis status, exact result, or Journey/Goal decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining book context. | **View analysis result** when complete |
| Exact analysis result | Show identity, quality, current and conditional evidence, provenance, and eligible deck preparation for one immutable result. | Prepare deck, return to book, or review scope |
| Deck preparation | Provide a coherent server-rendered status baseline before enhancement. | Download deck or return to book/Goal context |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock scoped
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Acquisition hub / catalog connections | Current `GET/POST /connections` and mutation routes | Add, edit, browse, or remove an owner-scoped OPDS connection. | Catalog browser | No connections, saved/credentialed connection, validation/authentication failure, deletion confirmation/error |
| Catalog browser | Current `GET /catalog` with `/opds/language`, `/opds/browse`, and `/opds/search` fragments | Find EPUBs in one ready analysis language without losing feed context. | Add to My Books or book detail | No connection, no ready language, capability discovery degraded, feed loading, breadcrumbs, pagination, search results, empty feed/search, upstream error |
| Catalog entry acquisition | Embedded current `POST /opds/acquire` result | Add one EPUB-backed book and continue browsing. | Remain in feed or open existing/new book | Adding, success, duplicate/idempotent existing book, unsupported/non-EPUB entry, download/validation failure |

The canonical design label is **Add to My Books** and must not imply analysis,
Journey membership, or Primary Goal selection. The shipped implementation uses **Add to My Books** for My Books membership (compatibility copy **Add to library** may still appear in older compatibility strings/tests); broader collection semantics for metadata-only books shipped under [ADR 0035](../../adr/0035-my-books-membership-and-source-provenance.md).

## Reading Journey and Primary Goal

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Reading Journey | Shipped `GET /journey` (compatibility `GET /campaigns` redirects to `/journey`); the queued campaign-queue/history surface is retained only as secondary Campaign history | Understand the current Primary Goal, freely shape a provisional order, and inspect current or conditional preparation evidence. | Primary Goal/book context, route comparison, My Books, or Where next? | Empty Journey, no Primary Goal, Goal with/without evidence, reorderable later books, unassessed/incomparable book, recalculating, recalculation failure, stale evidence, long content, narrow viewport |
| Route comparison and reorder preview | Embedded in Reading Journey | Compare **Your order** with an optional **Vocabulary-efficient alternative**, keep or adopt either, or make a manual change. | Updated Reading Journey | No comparable evidence, partial comparison, alternative available, manual preview, adopted change, neutral recalculation, failed recalculation |
| Primary Goal outcome / Where next? | Embedded transitional state in Reading Journey; route ownership requires planning | Understand what finishing the book actually changed and choose whether or where to commit next. | Choose as Primary Goal, reorder, My Books, continue vocabulary work, or no new Goal | Reading finished plus justified vocabulary transition, reading finished while vocabulary work remains, changed books, unchanged current evidence, no remaining Journey book, no next choice |
| Reading/preparation/vocabulary history | Secondary surface; current Campaign history may contribute after contract reconciliation | Review factual past reading, preparation, completion, abandonment, and vocabulary-transition events without restoring Campaign as principal IA. | Book or Journey context | Empty history, mixed historical states, legacy Campaign terminology, unavailable artifact |

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
The single-active Campaign and graduation contract currently conflicts with
parts of this target state; see the explicit planner/ADR boundary rather than
simulating the experience with copy.

## Settings and known vocabulary

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Settings | Current `GET /settings` and `/settings/languages` mutations | Manage study-language preferences and known vocabulary. | Remain in Settings | No profile, ready languages, saved language while capability discovery is degraded, no newly available languages, add/remove success or failure |
| Known-vocabulary import | Embedded in Settings; current `POST /known-vocab/import` and status endpoint | Upload one lemma per line and understand imported, duplicate, and rejected rows. | Updated known-vocabulary list | No selected language, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled |
| Direct known-vocabulary page | Current `GET /known-vocab` | Retained compatibility route that redirects to Settings. | Settings | Redirect to `/settings#known-vocabulary`; remaining states belong to embedded Settings |

Settings is the canonical destination. Under the current contract, removing a
study-language preference does not remove books, analyses, prepared artifacts,
internal Campaigns, or known vocabulary for that language. Journey/Goal effects
must not be claimed before those relationships receive a product contract.

## Inactive and supporting implementation

- `Dashboard` is an inactive template and not a canonical destination.
- Shipped navigation is **My Books** / **Reading Journey** / **Settings** plus distinct **Add books**; historical **My Library**, **Learning**, queue, and learner-facing Campaign labels remain only as compatibility fallbacks/redirects and are not the accepted target IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery and Stitch artifacts are evidence, not additional screen
  contracts.
