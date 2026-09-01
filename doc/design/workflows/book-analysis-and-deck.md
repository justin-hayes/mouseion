# Book analysis and deck workflow

Status: **Canonical shipped supporting workflow.** Analysis and deck preparation
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
Catalog browser
    -> Add to My Books
    -> Book detail
    -> Scope review
    -> Confirm scope
    -> Book detail
    -> Start analysis
    -> Analysis status
    -> Exact book-centered analysis result
    -> Prepare deck
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

### 2. Review and confirm scope

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

**Learner decision:** What does this exact result say about my readiness and the
vocabulary investment required for this book?

The canonical result route is
`/books/{book-id}/analyses/{analysis-run-id}`. It identifies its book, source,
confirmed scope, immutable completed analysis, and position in analysis history.
It never relies on a mutable “latest analysis” identity.

Information hierarchy answers, in order:

1. Can I trust this result, or are there quality warnings?
2. What is my current scoped coverage?
3. What additional vocabulary would reach the documented thresholds?
4. What structural signals add context without becoming a difficulty score?
5. Which provenance and history explain this exact result?
6. What can I do next?

Current, projected, scoped, token-weighted, and conditional numbers must be
labeled explicitly. Mouseion does not claim CEFR level, general reading level,
or a composite difficulty score.

Material quality warnings precede the evidence interpretation and deck action.
The interpretation may say that the learner's selected threshold is already
met, that a stated amount of vocabulary preparation would reach it, or that the
scope/evidence needs review. It does not decide whether the learner should read
the book. Deck preparation appears after the interpretation and core insights
and may be repeated as the closing action.

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
- Legacy/full-text analyses remain readable but do not unlock new scoped deck
  preparation.
- Degraded language-capability discovery preserves saved study preferences but
  blocks unsupported new operations.

## Shipped status-to-result transition

- keep `/jobs/{id}` for queued/running state, retry, cancellation, attempts, and
  failure recovery;
- completed scoped jobs expose **View analysis result**, rendered at
  `/books/{book-id}/analyses/{analysis-run-id}`;
- deck preparation is submitted from that exact result after its warnings and
  insights summary;
- list analysis history on the book page, linking active runs to status and
  completed runs to exact results.

The direct `POST /jobs/{id}/deck/preparations` action is retained only for
legacy compatibility and is not a canonical path for new scoped analyses.

Deck preparation may use JavaScript to consume its JSON status resource, but the
exact result and preparation status retain a coherent server-rendered baseline;
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
