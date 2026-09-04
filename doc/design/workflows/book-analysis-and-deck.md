# Book analysis and deck workflow

Status: **Canonical supporting workflow.** It includes the target
one-current-analysis transition proposed in
[ADR 0040](../../adr/0040-one-current-analysis-per-book.md), which remains
unshipped until its implementation issues land. Analysis and deck preparation
serve a book and, when present, its Primary Goal. Campaign remains a secondary
history/operations concept under ADRs 0027, 0034, and 0036; it is not a second
learner-facing plan.

## Goal

Help a learner move an owned EPUB from acquisition to a trustworthy, scoped
analysis and an optional prepared Anki deck without collapsing distinct
decisions into an automatic pipeline.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../features/explicit-scoped-analysis-workflow.md)
- [EPUB Analysis Scope Review](../../features/epub-analysis-scope-review.md)
- [Analysis Insights](../../features/analysis-insights.md)
- [ADR 0028: Explicit scoped-analysis lifecycle](../../adr/0028-explicit-scoped-analysis-lifecycle.md)
- [ADR 0022: Prepared decks](../../adr/0022-prepared-decks.md)
- [ADR 0040: One current analysis per book](../../adr/0040-one-current-analysis-per-book.md)

## Starting state and outcome

The primary path starts with an EPUB available in a learner-owned OPDS catalog.
It ends with either:

- an immutable completed analysis whose insights the learner can inspect; or
- an immutable ready APKG that may be downloaded and used in support of the
  book or its Primary Goal.

Neither outcome adds the book to Reading Journey, selects a Primary Goal, marks
reading complete, or marks vocabulary known. Campaign operations remain
secondary, and vocabulary graduation follows the single justified transition
defined by ADR 0036.

## Primary path

```text
Catalogue sync
    -> My Books
    -> Acquire EPUB content from Book detail
    -> Book detail
    -> Start analysis (entire book)
    -> Start analysis
    -> Analysis status
    -> Book detail with current analysis insights
    -> Prepare deck from book detail
    -> Deck-preparation status
    -> Download deck or return to the book / Primary Goal
```

### 1. Add to My Books

**Learner decision:** Which book do I want Mouseion to know about?

The canonical acquisition control says **Add to My Books**. It validates and
stores the EPUB source and extracted-unit snapshot, then updates the catalog
entry in place. It does not start analysis, confirm a scope, add the book to
Reading Journey, choose a Primary Goal, prepare a deck, or mark vocabulary
known.

Historical compatibility artifacts may say **Add to library**, but the shipped
acquisition control says **Add to My Books**.

The interface must answer:

- Was this EPUB added successfully?
- Was it already in My Books?
- Can I continue adding books without losing feed context?
- If acquisition failed, what can I do next?

### 2. Start analysis of the entire book

**Learner decision:** Which parts of this EPUB represent the text I intend to
read and analyze?

The scope page is a calm native checklist. A reliable top-level EPUB 3 TOC is
shown as one initially checked checkbox per top-level entry; each entry covers
its nested targets and expands to persisted readable unit IDs in spine order.
When the TOC cannot be projected into a complete, non-overlapping partition,
the page shows one initially checked checkbox per readable persisted unit in
flat spine order. The learner may check or uncheck choices and confirm an
immutable revision. Confirmation does not start analysis.

The interface must answer:

- What is currently selected, and how large is it?
- Is the page showing a reliable top-level TOC or the flat readable-unit
  fallback?
- Which persisted units will each checked choice include?
- What will confirmation do, and what will it not do?

My Books and book detail repeat the same learner-facing next action for this
state. The scope review itself makes **Confirm this scope** the next action and
states that confirmation returns the learner to the book page; it does not
start analysis.

For long books, the ordered checklist and selected-scope summary should remain
calm and scannable without turning the page into an evidence dashboard.

### 3. Start and monitor analysis

**Learner decision:** Am I ready to spend analysis resources on this exact
confirmed scope?

Submission creates or resolves an idempotent analysis run. The status experience
keeps the book title and confirmed scope in context while showing queued,
running, completed, failed, cancelled, and retrying state.

The interface must answer:

- Which book and scope are being analyzed?
- Is work queued, running, blocked, failed, cancelled, or complete?
- Is any action required?
- What will retry or cancellation do?
- Where will I inspect the completed result?

Operational identifiers, attempt counts, and timestamps are supporting detail;
they must not replace book identity and next-step guidance.

### 4. Inspect analysis insights

**Learner decision:** What does this book's current analysis say about my known
coverage and vocabulary investment, and do I want to prepare a deck?

The book page at `/books/{id}` is the sole learner-facing analysis surface. A
newly completed reanalysis replaces the current analysis on that page; earlier
runs remain operational audit records in `/jobs`. The compatibility route
`/books/{book-id}/analyses/{analysis-run-id}` redirects to the book page rather
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
preparation follows the retained evidence on the book page and may be repeated
as the closing action.

### 5. Prepare and download a deck

**Learner decision:** Do I want Mouseion to create study material from this
completed analysis, and do I consent to optional external sentence translation?

Deck preparation is asynchronous, owner-scoped, and tied to one immutable
completed analysis. The workflow reports durable progress, supports cancellation
and actionable retry, and publishes an immutable APKG. Download is a pure read.

The interface must answer:

- Which analysis will this deck use?
- What data leaves Mouseion if I enable optional translation?
- Is preparation queued, processing, waiting on Batch, failed, cancelled, or
  ready?
- Are any cards incomplete, retried, or excluded?
- Can I safely leave and return later?
- When ready, can I download the artifact and return to the book or Primary Goal?

## Alternate and edge paths

- Repeated OPDS acquisition resolves to the existing owned book.
- Failed acquisition never presents an invalid source as ready for scope review.
- Empty, stale, foreign, or contradictory scope submissions are rejected while
  preserving the learner's review context where possible.
- Metadata-only edits preserve scope identity; changed EPUB bytes require a new
  source snapshot and scope review.
- Duplicate analysis or preparation submission resolves idempotently rather than
  enqueuing competing work.
- Failed or cancelled analysis/preparation is retryable only when the server can
  establish viable work.
- Legacy/full-text analyses remain readable through operational audit paths but
  do not unlock new scoped deck preparation.
- Degraded language-capability discovery preserves saved study preferences but
  blocks unsupported new operations.

## Target status-to-result transition

- keep `/jobs/{id}` for queued/running state, retry, cancellation, attempts, and
  failure recovery;
- completed scoped jobs expose **View analysis result**, opening `/books/{id}`
  directly or through the run-specific compatibility redirect;
- deck preparation is submitted from the book page after its warning-only note
  and retained insights;
- do not list analysis history on the book page; `GET /jobs` remains the
  operational history surface; and
- a newly completed rerun replaces the book's current learner-facing analysis
  while prior immutable analyses remain operational audit records.

The direct `POST /jobs/{id}/deck/preparations` action is retained only for
legacy compatibility and is not a canonical path for new scoped analyses.

Deck preparation may use JavaScript to consume its JSON status resource, but the
book page and preparation status retain a coherent server-rendered baseline;
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
  primary action, then supporting provenance. Tables and scope evidence must not
  force page-level horizontal scrolling.
- Long-running analysis and Batch preparation must remain understandable when a
  learner leaves and returns later.
