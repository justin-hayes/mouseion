# Book analysis and deck workflow

## Goal

Help a learner move an owned EPUB from acquisition to a trustworthy, scoped
analysis and an optional prepared Anki deck without collapsing distinct
decisions into an automatic pipeline.

The product behavior is defined primarily by:

- [Explicit Scoped-Analysis Workflow](../../features/explicit-scoped-analysis-workflow.md)
- [EPUB Scope Workflows](../../features/epub-analysis-scope-workflows.md)
- [Analysis Insights](../../features/analysis-insights.md)
- [ADR 0028: Explicit scoped-analysis lifecycle](../../adr/0028-explicit-scoped-analysis-lifecycle.md)
- [ADR 0022: Prepared decks](../../adr/0022-prepared-decks.md)

## Starting state and outcome

The primary path starts with an EPUB available in a learner-owned OPDS catalog.
It ends with either:

- an immutable completed analysis whose insights the learner can inspect; or
- an immutable ready APKG that may be downloaded and added to the learning
  queue.

Neither outcome marks vocabulary known. Campaign completion is the graduation
boundary defined by ADR 0027.

## Primary path

```text
Catalog browser
    -> Add to library
    -> Book detail
    -> Scope review
    -> Confirm scope
    -> Book detail
    -> Start analysis
    -> Analysis status
    -> Completed analysis insights
    -> Prepare deck
    -> Deck-preparation status
    -> Download deck or add to learning queue
```

### 1. Add to library

**Learner decision:** Which book do I want to consider?

The acquisition control says **Add to library**. It validates and stores the
EPUB source and extracted-unit snapshot, then updates the catalog entry in place.
It does not start analysis, confirm a scope, prepare a deck, or mark vocabulary
known.

The interface must answer:

- Was this EPUB added successfully?
- Was it already in my library?
- Can I continue adding books without losing feed context?
- If acquisition failed, what can I do next?

### 2. Review and confirm scope

**Learner decision:** Which parts of this EPUB represent the text I intend to
read and analyze?

The recommendation is a review aid, not an automatic decision. The learner can
start from the recommendation, all readable units, or a prior scope; inspect
hierarchy, evidence, confidence, and warnings; override units; and confirm an
immutable revision. Confirmation does not start analysis.

The interface must answer:

- What is currently selected, and how large is it?
- Which units are uncertain or potentially structural noise?
- Why was each unit recommended or not recommended?
- Am I reusing or changing a historical decision?
- What will confirmation do, and what will it not do?

For long books, exception and low-confidence review should take visual priority
while complete evidence remains available through progressive disclosure.

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

The result identifies its source and confirmed scope. Information hierarchy
should answer, in order:

1. Can I trust this result, or are there quality warnings?
2. What is my current scoped coverage?
3. What additional vocabulary would reach the documented thresholds?
4. What structural signals or provenance explain the result?
5. What can I do next?

Current, projected, scoped, token-weighted, and conditional numbers must be
labeled explicitly. Mouseion does not claim CEFR level, general reading level,
or a composite difficulty score.

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
- When ready, can I download or add the deck to the learning queue?

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

## Current implementation gap

The current route after analysis submission is `/jobs/{id}`. On completion that
screen exposes deck preparation directly, while insights live on
`/books/{id}`. It does not provide a prominent analysis-result-to-insights
transition or consistently foreground the book title.

Until implementation changes, treat this as a known discontinuity:

- do not redefine the job page as the canonical insights screen;
- keep the accepted lifecycle as completion -> insights -> preparation;
- future work should preserve exact analysis identity while restoring that
  transition and parent-book context.

Deck preparation also relies on JavaScript to consume its JSON status resource.
A future workflow change should provide a coherent server-rendered baseline
before enhancement; status JSON is not itself a learner-facing page.

## Accessibility and responsive contract

- Use native headings, links, buttons, labels, fieldsets, legends, and
  checkboxes before introducing custom interaction semantics.
- Preserve a usable server-rendered initial state before HTMX or other
  JavaScript enhancement runs.
- Announce asynchronous state changes without repeatedly stealing focus.
- After a validation failure, expose the summary as an alert, move focus when
  appropriate, and preserve the learner's selections.
- Do not rely on color alone for recommendation, confidence, status, or error.
- At narrow widths, preserve the reading order: identity and state, explanation,
  primary action, then supporting provenance. Tables and scope evidence must not
  force page-level horizontal scrolling.
- Long-running analysis and Batch preparation must remain understandable when a
  learner leaves and returns later.
