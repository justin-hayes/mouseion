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
**Reading**, **Vocabulary**, **Catalog**, account identity, and **Log
out**. My Books, Reading, Vocabulary, and Catalog are the destinations.
Catalog keeps the built-in, read-only Classical catalog at `/catalogs` and one
sync status line per OPDS connection. The Catalog's one search and one language
filter cover the Classical catalog and every OPDS source, whose local Catalog
index entries appear as paged cover grids grouped by source. **Manage sources**
at `/catalogs/sources`
owns connection setup, credentials, editing, deletion, and sync maintenance. My Books is
the sole bibliographic Book browse surface for synced metadata; its items own
acquisition and analysis intent. The accepted Vocabulary target adds identity
Browse, not a second Book catalog.

The shell identifies the current destination, supports skip navigation and
keyboard use, and preserves a clear path back to the parent Book or Reading.
There is no Dashboard, Explore, Reading Horizon, or Learning destination in the
canonical learner-facing architecture.

The shell also carries a native **active study language** control on every
authenticated screen. It follows the four destinations in document and keyboard
order; wide screens place it at the trailing end, while compact screens show it
beside the wordmark above the destinations. It lists the learner's study
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
Book, it presents the active language's To Read candidates in neutral title
order, groups a Work's multiple To Read Books into one band-table entry, and
separates candidates with incomplete evidence. It does not recommend or rank
the next Book.

## Authentication

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| First-account onboarding | Current `GET /login`, `POST /onboarding` | Create the first local learner account. | My Books | Fresh installation, validation error, password requirements, submission failure |
| Sign in | Current `GET /login`, `POST /login` | Access an existing learner account. | Requested protected page or My Books | Invalid credentials, expired-session redirect, service error |

## My Books and book analysis

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| My Books | Shipped `GET /library`; catalogue browsing is reached through Catalog | Recognize and find a Book in the active language's cover-led collection, understand its one visible workflow bucket, inspect a grouped Work's Books, and inspect completion history. | Move to To Read, grouped Work bulk To Read or Remove the work, Remove from My Books (refused for the current Book), record a prior completion, read again, View in Reading, a Book cell, or Catalog | All, Inbox, To Read, and Read filters follow one visible bucket per Book; grouped Work identity, leading badge, counts, Book-order strip, one legend per page, single-Book position, whole-work no position; strip cells open the current-reading card or a To Read Book's inline chooser entry on Reading or the Switch page, while Read and Inbox cells stay status-only with their 44px cell and accessible label; filtered Work rows keep only matching cells and state "N Books in M works"; bulk To Read skips reading history and the current Book and prepares Books in book order on the classical queue; one grouped Work Analysis update notice and bulk action covering the row's visible eligible Books (search and filters narrow it with the cells), current-Book End wording and one confirmation with per-Book disclosures, while a single-Book Work keeps the ordinary notice; preparing Books collapse into one line while each problem Book keeps its own recovery; latest completion and total count; imported history distinct from Mouseion completion; empty disposition/history; metadata-only Book, cover states, needs-language text list, scoped search by author/English or Greek title/accent-insensitive with trailing Book number, paging, long content, enhancement unavailable; a To Read EPUB Book whose acquisition is queued or running shows the same in-progress label a classical Book shows, without **Retry**; a recorded EPUB acquisition failure names its reason on the To Read row and offers **Retry** only for entry, fetch and import stages, and the reason stays until an acquisition succeeds |
| Reading Book anchor | Embedded in `GET /reading`; legacy `/journey/{bookID}` is a compatibility redirect | Understand a Book's identity, current evidence, preparation state, and recovery actions without opening a competing detail page. | Focused deck preparation, analysis status, My Books, or Reading actions | Current/stale/unavailable evidence, queued/running/failed analysis, focused preparation state, current-reading state |
| Analysis status | Current `GET /jobs/{id}` with `GET /jobs/{id}/status` | Monitor, cancel, or retry one analysis run while retaining Book context. | Reading when complete | Queued, running, completed, failed/actionable, cancelled, retrying, historical result |
| Analysis history | Current `GET /jobs` | Inspect owner-scoped operational analysis history; this is not a learner result surface. | Individual analysis status or Reading | Empty history, mixed states, historical/legacy records |
| Exact-analysis compatibility route | Shipped `GET /books/{book-id}/analyses/{analysis-run-id}` redirect | Preserve deep links and references while opening the canonical Book anchor. | Reading anchor for current To Read Books; 404 otherwise | Valid owned current result redirect, non-member or incomplete result, missing or unauthorized reference |
| Deck preparation | Focused `GET /reading/books/{bookID}/deck/preparations/new`; current preparation mutation/status/download routes remain | Prepare an APKG from the Current reading's frozen snapshot (accepted target; shipped pre-reading path still exists), recover failure, and download the ready artifact. | Download deck or return to the Reading anchor | Queued, preparing by phase, long-running Batch, ready, failed/actionable, cancelled, retrying, completeness summary, and item-level contextual-meaning omissions with reasons |

My Books is the canonical home and a moderately dense bibliographic catalog.
The shipped [Book Covers](../features/book-covers.md) surface uses a responsive
grid led by the cover while title and author remain visible beneath it. My Books
communicates workflow placement and Read history; current reading and analysis
evidence remain concerns of Reading. Generic large cards and
metric-first sorting are not the default.

The My Books model includes metadata-only and currently unassessable works.
Each Book occupies exactly one visible workflow bucket, derived from current
reading, persisted disposition, and completion history. Current reading, the
persisted disposition, and history remain distinct facts; a historical Inbox
Book projects into Read without changing either underlying fact. Catalog sync
creates metadata-only Inbox Books; moving a Book to To Read enqueues the durable
EPUB acquisition when needed, and the Book carries its preparation state until
its analysis takes over. Metadata-only rows retain refresh and disposition
actions without opening a detail page.

### Analysis continuity

| Surface | Canonical responsibility | Primary exit |
|---|---|---|
| Reading Book anchor | Show Book identity, current-reading state, current evidence, and recovery. It does not render a generic Book detail, analysis-result, vocabulary-investment, or top-unknown page. | Analysis status, focused deck preparation, or Reading decision |
| Analysis status | Show queued/running progress, cancellation, retry, attempts, and actionable failure while retaining Book context. | **View in Reading** opens the canonical Book anchor when complete |
| Exact-analysis compatibility route | Redirect a valid current result URL to the Reading anchor; do not render a distinct insight, identity, provenance, or history surface. | Reading |
| Deck preparation | Provide a coherent focused task and server-rendered status baseline before enhancement. | Download deck or return to the Reading anchor |

Legacy/full-text jobs remain readable on `/jobs/{id}` but do not unlock current
deck preparation. The older `POST /jobs/{id}/deck/preparations` path is shipped
for compatibility but will cease new submissions under [ADR 0085](../adr/0085-reading-owned-book-vocabulary.md).
Historical classifier and
recommendation metadata is not a current learner-facing surface.

## Book acquisition

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Catalog | Current `GET /catalogs`; the Classical catalog index also remains at `GET /catalogs/classical` with one section per author and a work page at `GET /catalogs/classical/works/{work-key}`; retired `GET /catalogs/classical/authors/{author}` permanently redirects to that author's index section; an owned entry opens its Book through `GET /catalogs/books/{book-id}/in-my-books`; legacy `GET /connections` permanently redirects while POST connection mutation routes remain. | Find a text across the Classical catalog and every OPDS source with one search and one language filter, page an OPDS source's local Catalog index, open an owned Book in My Books, reach the Classical index, and see each source's sync state or Manage sources. | An owned Book in My Books, the Classical work page, the next result page, Sync now, Manage sources, or operational job status | One search and one language filter (defaulting to the active study language, or all languages when there is none) carried in real links and forms; results grouped by source; the Classical index keeps its author grouping, Openings, length strips, ownership and **Add and prepare** controls; each OPDS source shows its index entries as a cover grid with cover, title and author, paged in title-then-author order; entry ownership is **In My Books** linked to the Book, an unowned entry offers **Add** with copy saying it downloads and analyzes the text and a focused confirmation naming the Book and linking to My Books, plus the secondary **I've read this** that records the Book as Read without downloading or preparing it, and entries never show disposition, analysis or preparation state; search ignores accents and case through the Go search key, a trailing number narrows the Classical match to a constituent Book, and no search or browse contacts the OPDS server; distinct no-sources, no-match, nothing-in-this-language and nothing-to-browse-yet empty states; one result-summary live region; one status line per connection (never synced, syncing, last synced, sync failed) with Sync now and a failed message linked to Manage sources; withdrawn rows hidden and legacy-rows unavailability with its stable code; works without JavaScript, keyboard order and narrow layouts |
| Manage sources | Current `GET /catalogs/sources`; connection mutation routes at `POST /catalogs/sources`, `/catalogs/sources/{id}`, `/catalogs/sources/{id}/delete` and `/catalogs/sources/{id}/sync`, with the legacy `POST /connections` routes retained | Add, edit, remove, or synchronize an owner-scoped OPDS connection, and recover a failed sync. | Catalog, My Books, or operational job status | No connections with **Add your first catalog connection**, saved/credentialed connection, never synced, last synced, syncing, sync failed, validation failure, authentication failure, deletion confirmation/error |
| Book acquisition through reading intent | My Books disposition action, Catalog Add, and Catalog I've read this | Move a Book to To Read or Add a Catalog entry, acquire it when needed, and ensure current analysis once; or record an entry as already read without acquiring it. | Reading or analysis status | Adding/analyzing, success, duplicate/idempotent existing run, unsupported/non-EPUB entry, download/validation failure; a failed acquisition keeps the Book To Read, records the reason on its My Books row and offers Retry for entry, fetch and import stages but not for a refused source; a successful Retry clears the reason and starts analysis; I've read this creates or restores the Book as Read with a Previously read record and changes no Known vocabulary |

Moving a Book to To Read is the learner-facing acquisition and analysis trigger.
It acquires content when needed and ensures analysis without starting current
reading. Catalog sync remains metadata-only and follows metadata changes onto linked Books without ever invalidating or re-triggering analysis.

## Reading

| Screen | Current/target route | Learner goal | Primary exit | Required states |
|---|---|---|---|---|
| Current reading | Shipped `GET /reading` when a Book is active | Continue reading, inspect evidence and preparation, or explicitly end, switch, or finish the Book. | My Books, current-reading action, or finish receipt | Current/stale/unavailable evidence, queued/running/failed analysis, preparation state, stale form, each consequential confirmation |
| Current Book vocabulary Browse | Embedded in shipped `GET /reading` when a Book is current (working desk) | Find the current Book's effective lemma + POS identities, most frequent first, and open one in Concordance. | Concordance for the identity, or Reading recovery | Prefix search, 25-result pages, include-all, Known/Reserved/Book-deck states, no current analysis, empty inventory, prefix zero, counts unavailable, malformed or out-of-range `?page` with an explanation and First page link (400/404) |
| Between-Books chooser | Shipped `GET /reading` when no current Book exists | Choose among active-language To Read Books in neutral title order, with a Work's multiple Books grouped into one band table, without recommendations. | Start reading or My Books | Empty To Read collection, grouped Work identity/counts/coverage range, band order with omitted empty rows, per-band Book links and accessible labels, inline constituent entry, one grouped Work Analysis update notice and bulk action covering the visible To Read candidates while a single-Book Work keeps the ordinary notice, single-Book margin, analysis in progress, failed/stale/unavailable analysis, mixed and all-pending candidates |
| Occurrence lemma review | Shipped `GET /reading/books/{bookID}/lemma-review` | Find an exact observed form in the Book's current analysis, inspect each sentence and analyzer identity, and correct one occurrence without changing POS. | Return to the Reading Book anchor or follow explicit end/restart and new-generation deck recovery | No match, stale occurrence, unsupported language, active snapshot end-without-completion guidance, ready artifact download plus explicit re-preparation, completed-history/Known immutability and manual Anki cleanup guidance; server-rendered forms work without JavaScript |
| Reading completion receipt | `POST /goal/finish` full-page outcome or Reading fragment | Confirm the completed Book and exact newly-Known/already-Known counts without choosing a next Book. | Choose what to read next (`/reading`) | Non-empty and empty snapshots, zero counts, retry/idempotent completion, next-choice link |
| Read history | Filter on `GET /library?history=read` | Inspect prior completion for Books whose visible bucket is Read and return one to To Read with Read again. | My Books disposition or Reading | Imported vs Mouseion completion, multiple completions, empty history, language scope; historical Inbox Books appear in Read, while historical To Read Books stay in To Read |
| Historical artifact context | Supporting operational status and history surfaces | Inspect prepared-deck and legacy provenance without creating a separate learner workflow. | Reading anchor or focused preparation task | Empty history, preparing, ready, failed artifact, historical provenance |

Reading presents one current Book, with its vocabulary Browse, or, between Books,
an unordered semantic list of To Read candidates in neutral title order. A
Work's multiple To Read Books form one band-table entry; a constituent Book's
"Book N of M" position is a plain fact, and nothing on either surface points to
a next Book. Other To Read candidates are not listed while a Book is current.
Candidate order is not a recommendation. Coverage bands label current evidence
only. The shipped Book Covers experience keeps title and author authoritative
and does not change this hierarchy.

Starting, switching, ending, and finishing use separate
confirmations and stale-write protection. Keyboard order follows Book identity,
status, and actions; mutation feedback is announced. On narrow screens, identity
and primary actions remain visible without horizontal page scrolling.
Occurrence review is a focused Reading task rather than a competing Book detail
surface. Corrected identities participate in Book vocabulary insights and are
frozen into a new current-reading snapshot; the accepted deck path uses that
snapshot rather than independently reselecting. Changes remain unavailable once a reading snapshot is
active, preserving its historical vocabulary.

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
| Vocabulary Browse (shipped, working desk) | Embedded in `GET /reading` for the current Book | Discover effective lemma + POS identities in the Current reading Book, most frequent first. | Canonical-lemma prefix search; server-rendered 25-result pages | No study language, no Current reading, no current analysis, empty eligible inventory, prefix zero, recoverable query error, truthful Known/Reserved state |
| Retired known-vocabulary pages | `GET /known-vocab`, `GET`/`POST /vocabulary/import`, and `/vocabulary/imports/{id}/status` | None; the routes return 404 without a redirect (ADR 0092). | Vocabulary (`/vocabulary`) | 404 only; not a learner-facing screen |

Vocabulary is the canonical destination for Concordance,
scoped to the active study language; Browse is the working desk on Reading for the
current Book; its per-page language picker is removed in
favour of the shell-level switcher. Known-vocabulary-only languages (no current
chosen-language Book) remain selectable there as read-only "no books" entries.
Browse does not display
Known-only identities without current evidence. Changing a Book's language state does
not remove books, analyses, prepared artifacts, or historical vocabulary
provenance,
known vocabulary. The `/settings` compatibility route redirects to My Books; it is not a
learner-facing screen.

### Accepted Vocabulary target — partially shipped

Browse is hosted on Reading as the working desk for the Current reading Book and
ranks by descending effective occurrences in that Book, then across the learner's
current analyses. It retains canonical-lemma prefix search and a single include-all
control for Known, Reserved, and already-in-this-Book-deck identities. No Current
reading or stale analysis leads to a Reading recovery state; opening an identity
starts Concordance in that Book. Concordance is the Vocabulary landing at
`/vocabulary`, with no peer views.
Concordance supports cross-Book lookup, grammar exploration, KWIC, and focused
sentence study. The accepted, not-yet-implemented retirement removes Browse
selection and all saved Custom deck UI and HTTP access. See the [current Browse
and Concordance contract](../features/vocabulary-browse-and-concordance.md) and
[Reading-owned deck contract](../features/reading-owned-book-vocabulary.md).

| Screen | Learner goal and primary exit | Required states |
|---|---|---|
| Browse extensions | Open an identity in Concordance and inspect accounted-for words. | Identity handoff and include-all are shipped; Select/Remove and review are shipped but slated for removal |
| Concordance | Search exact surface, effective lemma + POS, or analyzer lemma + POS evidence; scan Books and open a focused study page for occurrence evidence. | Exact modes, applied multi-Book/grammar summary, 25-result KWIC paging, one uninterrupted occurrence list with quiet static page-local source labels (single-line visually ellipsized title, complete accessible text, no dividers or per-row repeats) and an always-visible trailing Study link, native click/Enter/Space sentence disclosure, exclusive named disclosures where supported, no custom row-key shortcuts, and narrow reflow are shipped |
| Focused sentence study | Study a complete sentence with Book/location and analyzer-attributed word evidence; return to the originating applied result. | The complete sentence, identified target, a source-aligned interlinear morphology line with a notation key, and a named complete text alternative (per-word surface, UPOS, analyzer/effective lemma, stored morphology features, relation/head, and correction/exclusion state, with unattributable occurrence decisions listed separately) are shipped, including local and whole-line fallback; explained missing parse and return to the originating result are shipped; the optional dependency diagram and per-word source-token evidence remain in later increments |

The accepted [Concordance rendering decision](../adr/0083-concordance-server-rendering-and-htmx-4.md)
ships one Templ-rendered result list, native grouped disclosures, no custom
row-key shortcuts, and pinned app-wide HTMX 4.0.0 for existing enhanced flows.
The handwritten Concordance lookup/history enhancement remains temporarily in
this intermediate slice; a follow-up moves it to HTMX.

## Inactive and supporting implementation

- `Dashboard` is an inactive template and not a canonical destination.
- Shipped navigation is **My Books** / **Reading** / **Vocabulary** /
  **Catalog**; historical **My Library**, **Learning**, queue, and learner-facing
  Campaign labels remain only as compatibility fallbacks/redirects and are not
  part of the canonical IA.
- Enrichment, deck-preparation, import, and recalculation status endpoints are
  supporting asynchronous resources, not global destinations.
- HTMX fragments and JSON responses must have a coherent parent screen and must
  not be treated as complete pages.
- Historical discovery documents are evidence, not additional screen
  contracts.
