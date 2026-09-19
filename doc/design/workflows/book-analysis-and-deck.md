# Book analysis and deck workflow

Status: **Canonical shipped supporting workflow.** Analysis and deck preparation
serve a Book and, when present, its Primary Goal. Historical Campaign records
remain supporting provenance under ADR 0072; the Goal owns the active frozen
vocabulary snapshot and the Journey entry owns its learner-facing context.
Reading Journey membership automatically ensures current
analysis under [ADR 0049](../../adr/0049-reading-intent-triggers-analysis.md),
with the standalone learner action retired by [ADR 0054](../../adr/0054-retire-standalone-analysis-action.md).

## Goal

Help a learner move a Book from catalog discovery to trustworthy current
analysis and an optional prepared Anki deck while keeping reading intent,
analysis refresh, insights, and deck preparation distinct.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../archive/features/explicit-scoped-analysis-workflow.md)
- [EPUB Analysis Scope Review](../../archive/features/epub-analysis-scope-review.md)
- [Analysis Insights](../../features/analysis-insights.md)
- [ADR 0028: Explicit scoped-analysis lifecycle](../../adr/0028-explicit-scoped-analysis-lifecycle.md)
- [ADR 0022: Prepared decks](../../adr/0022-prepared-decks.md)
- [ADR 0040: One current analysis per book](../../adr/0040-one-current-analysis-per-book.md)
- [ADR 0049: Reading intent triggers analysis](../../adr/0049-reading-intent-triggers-analysis.md)
- [ADR 0054: Retire the standalone analysis action](../../adr/0054-retire-standalone-analysis-action.md)

## Starting state and outcome

The primary path starts with a metadata-only or acquired Book in My Books.
It ends with either:

- an immutable completed analysis whose insights the learner can inspect; or
- an immutable ready APKG that may be downloaded and used in support of the
  Book or its Primary Goal.

Adding a Book to Reading Journey is the sole learner-initiated acquisition and
analysis path: it retains membership, acquires the current EPUB when needed, and
ensures one whole-book analysis for the current content revision. Analysis never
selects a Primary Goal, marks reading complete, or marks vocabulary Known.
Choosing a Goal freezes the snapshot; accepted Goal completion adds it to
modeled Known vocabulary independently of deck readiness.

## Primary path

```text
Catalog sync
    -> My Books
    -> Add to Reading Journey
    -> Acquire current EPUB and ensure whole-book analysis
    -> Analysis status
    -> Journey entry with current evidence
    -> Choose Goal and freeze snapshot, or prepare an artifact from the entry
    -> Download deck or return to the Journey / Primary Goal
```

Adding a Book to Reading Journey from My Books is the
learner-initiated reading-intent path. It retains Journey membership first,
acquires the current EPUB when needed, and submits whole-book analysis.
Re-adding a member reuses current completed or in-flight work; reordering never
performs either side effect.

### 1. Discover a Book in My Books

**Learner decision:** Which Book do I want to inspect or consider reading?

Catalog sync creates or updates the metadata-only My Books entry. It does not
download content, trigger analysis, add the Book to Reading Journey, choose a
Primary Goal, prepare a deck, or mark vocabulary known.

The interface must answer:

- Is this Book metadata-only or acquired?
- Is current analysis available, queued, running, stale, failed, or absent?
- Is this the Book I want to add to the Reading Journey?

No detail page is offered before a current completed analysis exists. Analysis
status and recovery live on the Jobs page and inline on Journey cards.

### 2. Monitor analysis

**Learner decision:** What is the state of the analysis that Reading Journey
membership ensured?

Submission creates or resolves an idempotent analysis run. The status experience
keeps the Book title and current content revision in context while showing queued,
running, completed, failed, cancelled, and retrying state.

The interface must answer:

- Which Book and content revision are being analyzed?
- Is work queued, running, blocked, failed, cancelled, or complete?
- Is any action required?
- What will retry or cancellation do?
- Where will I inspect the completed result?

Operational identifiers, attempt counts, and timestamps are supporting detail;
they must not replace book identity and next-step guidance.

### 4. Inspect analysis insights

**Learner decision:** What does this book's current analysis say about my known
coverage and vocabulary investment, and do I want to prepare a deck?

The current completed analysis is presented in the Journey entry at
`/journey/{bookID}`. A newly completed reanalysis
replaces the current analysis; earlier runs remain operational audit records in
`/jobs`. The compatibility route `/books/{book-id}/analyses/{analysis-run-id}`
redirects to the Journey entry for members and returns 404 otherwise, rather
than presenting a separate exact-result experience.

Information hierarchy answers, in order:

1. What is my **Current known coverage** of the analyzed units?
2. What additional vocabulary would reach the documented thresholds?
3. Which unknown vocabulary has the highest contribution?
4. Does a concrete analyzer or data-integrity gap require a warning?
5. Do I want to prepare a deck?

Current known coverage is the headline and premier metric. Its one-line scope
qualifier, such as “of the analyzed units,” is a caption rather than an
**Analyzed scope** section. **Vocabulary investment** / **Additional
vocabulary** and **Highest-impact unknown vocabulary** follow. Render one
compact analysis-quality note only when a reproducible warning exists; render
nothing for a clean analysis.

Do not render analyzed-scope details, text profile, projected token coverage,
the rest of the former coverage-stat list, analysis history, identity/trust, or
provenance/history sections on the learner surface. Mouseion does not claim
CEFR level, general reading level, or a composite difficulty score. Deck
preparation follows the retained evidence on the Journey entry and may be repeated
as the closing action.

### 5. Prepare and download a deck

For a manual preparation from a Journey entry, the learner decides whether to
create study material and whether to consent to optional external sentence
translation. Selecting a Primary Goal is different: Mouseion automatically
prepares local study material from that Goal's immutable vocabulary snapshot;
Goal selection never grants external translation consent.

Deck preparation is asynchronous, owner-scoped, and tied to one immutable
completed analysis or one immutable Goal snapshot. The workflow reports durable
progress, supports cancellation and actionable retry, and publishes a ready APKG
whose artifact may later be superseded in place by a newer deck revision.
Download is a pure read and serves the current artifact. Empty Goal snapshots
remain valid Goals and do not require a deck artifact; failed or unavailable
Goal artifacts remain visible without clearing the Goal or its snapshot.

The interface must answer:

- Which analysis will this deck use?
- What data leaves Mouseion if I enable optional translation?
- Is preparation queued, processing, waiting on Batch, failed, cancelled, or
  ready?
- Are any cards incomplete, retried, or excluded?
- Can I safely leave and return later?
- When ready, can I download the artifact and return to the book or Primary Goal?
- Is a newer deck revision available, and will download serve that current revision?

## Alternate and edge paths

- Repeated OPDS acquisition resolves to the existing owned book.
- Failed acquisition never presents an invalid source as ready for analysis.
- Empty or unavailable acquisition states preserve Book/Journey context and offer
  recovery without treating membership as failed.
- A completed preparation with no recurring vocabulary shows an explicit empty
  state explaining that there are no cards to study; it does not offer an empty
  artifact download or a separate study action. A zero-card preparation with
  quality omissions remains a completeness result and keeps its artifact action.
- Metadata-only edits preserve current evidence; changed EPUB bytes create a
  stale current-analysis state until the learner re-analyzes from Reading Journey.
- Duplicate analysis or preparation submission resolves idempotently rather than
  enqueuing competing work.
- Failed or cancelled analysis/preparation is retryable only when the server can
  establish viable work.
- Legacy/full-text analyses remain readable through operational audit paths but
  do not unlock new scoped deck preparation.
- Degraded language-capability discovery does not erase derived language context
  or stored display names, but blocks unsupported new operations.

## Target status-to-result transition

- keep `/jobs/{id}` for queued/running state, retry, cancellation, attempts, and
  failure recovery;
- completed analysis jobs expose **View analysis result**, opening `/journey/{bookID}`
  directly or through the run-specific compatibility redirect;
- manual deck preparation is submitted from the Journey entry after its
  warning-only note and retained insights; Primary Goal selection submits a
  local preparation from the exact frozen Goal snapshot;
- do not list analysis history on the Journey entry; `GET /jobs` remains the
  operational history surface; and
- a newly completed rerun replaces the book's current learner-facing analysis
  while prior immutable analyses remain operational audit records.

The direct `POST /jobs/{id}/deck/preparations` action is retained only for
legacy compatibility and is not a canonical path for new analyses.

Deck preparation may use JavaScript to consume its JSON status resource, but the
Journey entry and preparation status retain a coherent server-rendered baseline;
status JSON is not itself a learner-facing page.

## Accessibility and responsive contract

- Use native headings, links, buttons, labels, fieldsets, legends, and
  checkboxes before introducing custom interaction semantics.
- Preserve a usable server-rendered initial state before HTMX or other
  JavaScript enhancement runs.
- Announce asynchronous state changes without repeatedly stealing focus.
- After a validation failure, expose the summary as an alert, move focus when
  appropriate, and preserve the learner's selections.
- Do not rely on color alone for checklist state, status, or error.
- At narrow widths, preserve the reading order: identity and state, explanation,
  primary action, then supporting provenance. Tables and analysis evidence must not
  force page-level horizontal scrolling.
- Long-running analysis and Batch preparation must remain understandable when a
  learner leaves and returns later.
