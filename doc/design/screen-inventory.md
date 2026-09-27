# Screen inventory

Status: **Canonical shipped learner-facing screen contract, including the
Reading workflow and Book cover behavior.** ADR 0074's Journey terminology and
ADR 0072's Goal snapshot/forecast behavior are historical; active screens use
current reading, To Read, and independent Read history.
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
study language scopes My Books, Reading, and Vocabulary; current reading is
scoped by language. Catalog maintenance is a fourth principal
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
**Reading**, **Vocabulary**, **Catalogs**, account identity, and **Log
out**. My Books, Reading, Vocabulary, and Catalogs are the destinations.
Catalogs owns catalogue setup and sync maintenance at `/catalogs`. My Books is
the sole browse surface; its items own acquisition and analysis intent.

The shell identifies the current destination, supports skip navigation and
keyboard use, and preserves a clear path back to the parent Book or Reading.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shell also carries a native **active study language** control alongside the
four destinations, on every authenticated screen. It lists the learner's study
languages plus any known-vocabulary-only language marked "no books", marks a
newly arrived study language "new", and is keyboard-accessible and
server-rendered before enhancement. Changing it navigates to the same screen in
the new language on language-scoped screens and updates the stored mode
elsewhere; it never auto-switches on navigation or sync.

The shipped application routes `/` to `/library` and serves Reading at `/reading`.
Legacy `/journey` URLs redirect to Reading; former Journey anchors do not expose
a separate Book or analysis view. Focused deck preparation and operational
history remain supporting surfaces. No learner-facing ordered queue, campaign,
or plan is exposed.

The between-Books chooser is shipped at `GET /reading`: when there is no current
Book, it presents the active language's To Read candidates in lexical coverage
bands and separates candidates with incomplete evidence. It does not recommend
or rank the next Book.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Shipped `GET /library`; catalogue browsing is reached through Catalogs | Recognize and find a Book in the active language's cover-led collection, understand its one visible workflow bucket, and inspect completion history. | Move to To Read, set aside, record a prior completion, read again, View in Reading, Catalogs, or Refresh metadata; setting aside a historical Book returns to Read; there is no ordinary Remove from My Books action | All, Inbox, To Read, Set Aside, and Read filters follow one visible bucket per Book; latest completion and total count; imported history distinct from Mouseion completion; empty disposition/history; metadata-only Book, cover states, needs-language text list, scoped search, paging, long content, enhancement unavailable |
| Reading Book anchor | Embedded in `GET /reading`; legacy `/journey/{bookID}` is a compatibility redirect | Understand a Book's identity, current evidence, preparation state, and recovery actions without opening a competing detail page. | Focused deck preparation, analysis status, My Books, or Reading actions | Current/stale/unavailable evidence, queued/running/failed analysis, focused preparation state, current-reading state |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining Book context. | Reading when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or Reading | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Shipped `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the canonical Book anchor. | Reading anchor for current To Read Books; 404 otherwise | Valid owned current result redirect, non-member or incomplete result, missing or unauthorized reference |
| Deck preparation | Focused `GET /reading/books/{bookID}/deck/preparations/new`; current preparation mutation/status/download routes remain | Consent to optional translation, prepare an APKG from the current analysis, recover failure, and download the ready artifact. | Download deck or return to the Reading anchor | Consent absent/present, queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, cleanup warning, completeness summary |

My Books is the canonical home and a moderately dense bibliographic catalog.
The shipped [Book Covers](../features/book-covers.md) surface uses a responsive
grid led by the cover while title and author remain visible beneath it. My Books
communicates workflow placement and Read history; current reading and analysis
evidence remain concerns of Reading. Generic large cards and
metric-first sorting are not the default.

The My Books model includes metadata-only and currently unassessable works.
Each Book occupies exactly one visible workflow bucket, derived from current
reading, persisted disposition, and completion history. Current reading, the
persisted disposition, and history remain distinct facts; a historical Set Aside
Book projects into Read without changing either underlying fact. Catalog sync
creates metadata-only Inbox Books; moving a Book to To Read acquires and validates
EPUB content when needed. Metadata-only rows retain refresh and disposition
actions without opening a detail page.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Reading Book anchor | Show Book identity, current-reading state, current evidence, and recovery. It does not render a generic Book detail, analysis-result, vocabulary-investment, or top-unknown page. | Analysis status, focused deck preparation, or Reading decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining Book context. | **View in Reading** opens the canonical Book anchor when complete |
| Exact-analysis compatibility route | Redirect a valid current result URL to the Reading anchor; do not render a distinct insight, identity, provenance, or history surface. | Reading |
| Deck preparation | Provide a coherent focused task and server-rendered status baseline before enhancement. | Download deck or return to the Reading anchor |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock current
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is retained
for compatibility and is not a competing destination. Historical classifier and
recommendation metadata is not a current learner-facing surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalogs | Current `GET /catalogs`; legacy `GET /connections` permanently redirects while POST connection mutation routes remain | Add, edit, remove, or synchronize an owner-scoped OPDS connection. | Sync now, My Books, or operational job status | No connections, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation/authentication failure, deletion confirmation/error |
| Book acquisition through reading intent | My Books disposition action | Move a Book to To Read, acquire it when needed, and ensure current analysis once. | Reading or analysis status | Adding/analyzing, success, duplicate/idempotent existing run, unsupported/non-EPUB entry, download/validation failure |
| Per-book catalog metadata refresh | Current item action on `GET /library` with `POST /library/books/{id}/refresh` | Refresh one catalog-backed Book's metadata without downloading EPUB content or changing evidence. | My Books item | Refreshing, updated, unchanged, upstream entry missing/no-op, connection failure; cover pending/unavailable; no scope or analysis invalidation |

Moving a Book to To Read is the learner-facing acquisition and analysis trigger.
It acquires content when needed and ensures analysis without starting current
reading. Catalog sync remains metadata-only, and My Books item
metadata refresh never invalidates or re-triggers analysis.

## Reading

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Current reading | Shipped `GET /reading` when a Book is active | Continue reading, inspect evidence and preparation, or explicitly stop, set aside, switch, or finish the Book. | My Books, current-reading action, or finish receipt | Current/stale/unavailable evidence, queued/running/failed analysis, preparation state, stale form, each consequential confirmation |
| Between-Books chooser | Shipped `GET /reading` when no current Book exists | Choose among active-language To Read Books using current coverage bands without recommendations. | Start reading or My Books | Empty To Read collection, coverage bands, analysis in progress, failed/stale/unavailable analysis, mixed and all-pending candidates |
| Reading completion receipt | `POST /goal/finish` full-page outcome or Reading fragment | Confirm the completed Book and exact newly-Known/already-Known counts without choosing a next Book. | Choose what to read next (`/reading`) | Non-empty and empty snapshots, zero counts, retry/idempotent completion, next-choice link |
| Read history | Filter on `GET /library?history=read` | Inspect prior completion for Books whose visible bucket is Read and return one to To Read with Read again. | My Books disposition or Reading | Imported vs Mouseion completion, multiple completions, empty history, language scope; historical Inbox Books appear in Read, while historical To Read Books stay in To Read |
| Historical artifact context | Supporting operational status and history surfaces | Inspect prepared-deck and legacy provenance without creating a separate learner workflow. | Reading anchor or focused preparation task | Empty history, preparing, ready, failed artifact, historical provenance |

Reading presents one current Book or an unordered semantic list of To Read
candidates. Candidate order is not a recommendation. Coverage bands label
current evidence only. The shipped Book Covers experience keeps title and author
authoritative and does not change this hierarchy.

Starting, switching, stopping, setting aside, and finishing use separate
confirmations and stale-write protection. Keyboard order follows Book identity,
status, and actions; mutation feedback is announced. On narrow screens, identity
and primary actions remain visible without horizontal page scrolling.

### Reading completion semantics

When current reading is finished:

1. acknowledge **Reading finished** and end the Book's current-reading state;
2. identify the completed Book and state the exact newly-Known and already-Known
   identity counts, including zero for an empty snapshot;
3. offer **Choose what to read next** and return to the `/reading` chooser without
   selecting another Book.

The receipt is intentionally restrained. Do not say a Journey is complete,
automatically choose another Book, call a Book optimal, or claim vocabulary
gains when the transition has not occurred. Completion history remains separate
from the My Books disposition, and Read again preserves earlier completions.

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
- Shipped navigation is **My Books** / **Reading** / **Vocabulary** /
  **Catalogs**; historical **My Library**, **Learning**, queue, and learner-facing
  Campaign labels remain only as compatibility fallbacks/redirects and are not
  part of the canonical IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery and Stitch artifacts are evidence, not additional screen
  contracts.
