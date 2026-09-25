# Screen inventory

Status: **Canonical shipped learner-facing screen contract, including Book cover
behavior.** ADR 0074 ships the consolidated Reading
Journey Book surface and assigns deck work to the focused preparation task. ADR
0072 continues to own the Goal snapshot and forecast semantics described below.
It includes the
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
out**. My Books, Reading Journey, Vocabulary, and Catalogs are the destinations.
Catalogs owns catalogue setup and sync maintenance at `/catalogs`. My Books is
the sole browse surface; its items own acquisition and analysis intent.

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
`/journey`. A former `/journey/{bookID}` bookmark now redirects an authorized,
reachable Book to its canonical Reading Journey anchor; it does not expose a
separate Book or analysis view. Focused deck preparation and operational history
remain supporting surfaces. No parallel learner-facing queue, campaign, or plan
is exposed.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Shipped `GET /library`; catalogue browsing is reached through Catalogs | Recognize and find a Book in the active language's cover-led collection, understand its Inbox, To Read, or Set Aside placement, distinguish Reading Journey membership, and inspect reading history independently. | Move to To Read, set aside, record a prior completion, read again, View in Reading Journey, Catalogs, Refresh metadata, or Remove from My Books | All, Inbox, To Read, Set Aside, and Read history filters; latest completion and total completion count; imported history distinct from Mouseion completion; empty disposition/history; metadata-only Book, valid cover, no cover, retrieval pending, replacement pending, cover unavailable, in/out of Journey, needs-language text list, scoped search, paging, no match, later page removed, long content, enhancement unavailable |
| Reading Journey book anchor | Embedded in `GET /journey`; `/journey/{bookID}` is a compatibility redirect | Understand a Book's identity, supporting cover thumbnail, Journey/Goal relationship, current evidence, forecast, and recovery actions without opening a competing detail page. | Focused deck preparation, analysis status, My Books, or Journey actions | Valid, missing, pending, unavailable, or retained replacement cover; current/stale/unavailable evidence, queued/running/failed analysis, focused preparation state, reading and Goal state |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining book context. | Reading Journey when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or Reading Journey | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Shipped `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the canonical Book anchor. | Reading Journey anchor for members; 404 otherwise | Valid owned member/run redirect, non-member or incomplete result, missing or unauthorized reference |
| Deck preparation | Focused `GET /journey/books/{bookID}/deck/preparations/new`; current preparation mutation/status/download routes remain | Consent to optional translation, prepare an APKG from the current analysis, recover failure, and download the ready artifact. | Download deck or return to the Reading Journey anchor | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalog.
The shipped [Book Covers](../features/book-covers.md) surface uses a responsive
grid led by the cover while title and author remain visible beneath it. My Books
communicates workflow placement and Reading Journey membership; Primary Goal and
analysis evidence remain concerns of Reading Journey. Generic large cards and
metric-first sorting are not the default.

The My Books model includes metadata-only and currently unassessable works.
Each Book occupies exactly one workflow disposition, while reading completion
remains an independent history projection. Catalog sync creates metadata-only
membership; adding a Book to Reading Journey acquires and validates EPUB content
when needed. Metadata-only rows retain refresh, disposition, Journey, and removal
actions without opening a detail page.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Reading Journey book anchor | Show Book identity, Journey/Goal relationship, current evidence, forecast, and evidence recovery. It does not render a generic Book detail, analysis-result, vocabulary-investment, or top-unknown page. | Analysis status, focused deck preparation, or Journey/Goal decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining Book context. | **View in Reading Journey** opens the canonical Book anchor when complete |
| Exact-analysis compatibility route | Redirect a valid current member result URL to the Reading Journey anchor; do not render a distinct insight, identity, provenance, or history surface. | Reading Journey |
| Deck preparation | Provide a coherent focused task and server-rendered status baseline before enhancement. | Download deck or return to the Reading Journey anchor |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock current
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination. Historical classifier and
recommendation metadata is not a current learner-facing surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalogs | Current `GET /catalogs`; legacy `GET /connections` permanently redirects while POST connection mutation routes remain | Add, edit, remove, or synchronize an owner-scoped OPDS connection. | Sync now, My Books, or operational job status | No connections, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation/authentication failure, deletion confirmation/error |
| Book acquisition through reading intent | Current `POST /journey/books/{id}/add`, launched from My Books or deck status | Express intent, acquire one My Books Book when needed, and ensure current analysis once. | Journey or analysis status | Adding/analyzing, success, duplicate/idempotent existing run, unsupported/non-EPUB entry, download/validation failure |
| Per-book catalog metadata refresh | Current item action on `GET /library` with `POST /library/books/{id}/refresh` | Refresh one catalog-backed Book's metadata without downloading EPUB content or changing evidence. | My Books item | Refreshing, updated, unchanged, upstream entry missing/no-op, connection failure; cover pending/unavailable; no scope or analysis invalidation |

Adding a Book to Reading Journey is the sole learner-facing acquisition and
analysis trigger. It acquires content when needed and ensures analysis without
selecting a Primary Goal. Catalog sync remains metadata-only, and My Books item
metadata refresh never invalidates or re-triggers analysis.

## Reading Journey and Primary Goal

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Reading Journey | Shipped `GET /journey` | Express reading intent, understand the current Primary Goal, freely shape a provisional order, and inspect current, after-Goal, and on-arrival evidence for the active language. | Primary Goal/book context, My Books, or Where next? | Empty Journey, no Primary Goal, queued/running/current analysis, unavailable evidence, recalculating, recalculation failure, stale evidence, lower-bound forecast, long content, narrow viewport |
| Journey forecast and reorder preview | Embedded in Reading Journey | See the three labeled coverage meanings in **Your order** and make a manual change within the active language's Journey. | Updated Reading Journey | No Goal, active Goal snapshot, unavailable predecessor, lower-bound forecast, neutral recalculation, failed recalculation |
| Primary Goal outcome / Where next? | Embedded target state in Reading Journey | Understand the exact modeled vocabulary and reading changes from completing the Goal, then choose whether or where to commit next. | Choose as Primary Goal, reorder, My Books, or no new Goal | Goal completion with non-empty or empty snapshot, changed forecasts, lower-bound evidence, no remaining Journey book, no next choice; one Goal per language, other languages' Goals unaffected |
| Historical artifact context | Supporting operational status and history surfaces | Inspect prepared-deck and legacy provenance without creating a separate learner workflow. | Reading Journey anchor or focused preparation task | Empty history, preparing, ready, failed artifact, historical provenance |

The Reading Journey is an ordered semantic list. The Primary Goal is anchored
above the provisional books. The learner's order remains canonical. Current,
after-Goal, and on-arrival coverage are clearly labeled evidence; unavailable
predecessors make downstream values lower bounds rather than fabricated zeros.
The shipped Book Covers experience reserves one modest aligned thumbnail for the
Primary Goal and every provisional item without changing this hierarchy.

Visible **Move earlier** and **Move later** controls are required. Drag may
enhance them. Reordering retains focus, announces the new position, and reports
forecast recalculation in a scoped live region. On narrow screens, labeled
forecast values stack without changing order or hiding authors and evidence.

### Goal outcome semantics

When the Primary Goal is completed:

1. acknowledge **Reading finished** and end the book's current Primary Goal role;
2. state the exact number of frozen snapshot identities added to modeled Known
   vocabulary, including zero for an empty snapshot;
3. recalculate later forecasts from actual modeled state;
4. show precise changed or unchanged evidence;
5. ask **Where next?** without creating another Goal.

Do not say the Journey is complete, automatically choose another Goal, call a
book optimal, or claim vocabulary gains when the transition has not occurred.
Legacy Campaign records remain historical provenance only. ADR 0072 governs the
Goal-owned snapshot, completion transition, and forecast shown by this surface.
Goals are one per study language (ADR 0051): finishing a Goal in the active
language does not touch other languages' Goals.

## Active study language, derived study languages, and known vocabulary

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Vocabulary | `GET /vocabulary`, `POST /vocabulary/import`, and import status endpoint | Import known vocabulary for the active study language: upload one lemma per line and understand imported, duplicate, and rejected rows. | Import result summary | No derived study languages, no file, invalid file type, queued/processing, complete, partial rejection, failed, cancelled; scoped to the active language with no per-page picker and no known-vocabulary list |
| Direct known-vocabulary page | Compatibility `GET /known-vocab` | Redirect to the Vocabulary destination. | Vocabulary | Redirect to `/vocabulary`; remaining states belong to Vocabulary |

Vocabulary is the canonical destination for known-vocabulary import, scoped to the
active study language; its per-page language picker is removed in favour of the
shell-level switcher. Known-vocabulary-only languages (no current chosen-language
Book) remain selectable there as read-only "no books" entries; import stays
limited to the derived study-language set. The page does not display the
known-vocabulary read model. Changing a Book's language state does
not remove books, analyses, prepared artifacts, or historical vocabulary
provenance,
known vocabulary. The `/settings` compatibility route redirects to My Books; it is not a
learner-facing screen.

## Inactive and supporting implementation

- `Dashboard` is an inactive template and not a canonical destination.
- Shipped navigation is **My Books** / **Reading Journey** / **Vocabulary** /
  **Catalogs**; historical **My Library**, **Learning**, queue, and learner-facing
  Campaign labels remain only as compatibility fallbacks/redirects and are not
  part of the canonical IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery and Stitch artifacts are evidence, not additional screen
  contracts.
