# Explicit scoped-analysis workflow

Status: Implemented · Date: 2026-08-26 · Updated: 2026-09-01

Shipped labels are **My Books** and **Add to My Books** (acquisition creates or restores My Books membership). Historical **My Library** / **Add to library** copy is retained only as a compatibility note; each acquired entry maps to an acquired Book with active My Books
membership under
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md). That
compatibility mapping does not mean every My Books Book has an EPUB; metadata-only
membership cannot enter the scope, analysis, or preparation lifecycle below.

## Goal

Let learners collect EPUBs, decide exactly what to analyze, inspect completed
results, and prepare a deck through distinct, durable, reproducible steps.

## Learner journey

1. Connect a learner-owned catalogue from the My Books empty state, choose a
   ready catalogue language, and synchronize metadata.
2. Browse the local My Books collection and open a metadata-only Book.
3. Acquire that Book's EPUB content without starting analysis.
4. Review its extracted units and confirm a scope revision.
5. Explicitly start asynchronous analysis for that confirmed scope.
6. Observe queued, running, completed, failed, or cancelled status; retry a
   failed or cancelled run when allowed.
7. Inspect the book's single current analysis on the book page.
8. Request asynchronous deck preparation there; the request remains bound to
   that completed analysis internally.
9. Observe preparation status and download the immutable ready APKG.

The identity and state contracts for these resources are normative in
[ADR 0028](../adr/0028-explicit-scoped-analysis-lifecycle.md).

## My Books and content acquisition

- Each acquisition control says **Add to My Books**, not **Import & analyze**. Historical **Add to library** copy may still appear in older screenshots or compatibility strings.
- My Books owns local language grouping, search, paging, and Book selection.
- A metadata-only Book offers explicit per-book acquisition from its detail page;
  the form posts to `/opds/acquire` with an owner-scoped signed target.
- Repeated acquisition of the same owner, catalog entry, and source content is
  idempotent and reports the existing My Books Book.
- Acquisition downloads and validates the EPUB, persists immutable source
  content and extracted-unit identity, and performs no NLP analysis.
- Partial or failed acquisition is actionable and does not display the book as
  ready for scope review until its source snapshot is valid.

## Scope review and confirmation

- Scope review presents reliable top-level EPUB 3 TOC entries as initially
  checked checkboxes, expanding each entry to persisted readable units in
  spine order. When a complete mapping is not reliable, it presents one
  initially checked checkbox per readable persisted unit in flat spine order.
  The detailed projection and fallback contract is owned by [EPUB analysis
  scope review](epub-analysis-scope-review.md).
- Nested TOC entries are covered by their top-level parent and are not separate
  controls. **Check all** and **Uncheck all** affect every rendered choice;
  confirmation requires at least one readable persisted unit.
- Saving a confirmed scope does not start analysis.
- A confirmed scope page identifies its revision, source-content digest/revision,
  unit snapshot, canonical ordered selected-unit references, and selection
  summary. It does not display classifier or recommendation provenance.
- Changing a selection creates a new revision. Metadata-only title, author, or
  language edits do not invalidate a scope because they do not replace content.
- If actual EPUB bytes change in a future supported flow, the book returns to
  **scope review required** for the new content revision.

## Explicit asynchronous analysis

- Only a confirmed, owner-scoped scope can be submitted.
- Submission returns a durable analysis-run identity and status URL.
- Duplicate submission for the same logical request resolves to the same run;
  concurrent requests do not enqueue competing work.
- Status shows the current state, attempt count, timestamps, and an actionable
  failure without exposing sensitive provider or source data.
- Retry atomically creates or identifies a viable River attempt. A queued or
  running status without viable work is reconciled rather than polled forever.
- Completion publishes one immutable analysis artifact with scope and source
  provenance. A newly completed rerun replaces the book's current
  learner-facing analysis; earlier artifacts remain operational audit records
  under [ADR 0040](../adr/0040-one-current-analysis-per-book.md).

## Deck preparation prerequisite

- The prepare action is available on the book page only when a current analysis
  is completed. It submits that exact analysis ID internally, not a mutable
  book-level analyzed flag.
- The server validates the entire owner-scoped book → source → scope → analysis
  chain and rejects incomplete, failed, stale, legacy-only, or cross-owner input.
- Preparation retry and reconciliation follow ADR 0028 and preserve ADR 0022's
  immutable ready-artifact and pure-download guarantees.
- Deck preparation does not mark vocabulary known. Vocabulary graduation uses the single justified transition of [ADR 0036](../adr/0036-primary-goal-justified-graduation.md); reading-finished alone graduates nothing. Campaign operations remain secondary/history only.

## Compatibility and rollout

- Existing completed analyses remain readable as operational audit records, and
  prepared decks remain downloadable with their historical labels. The
  run-specific learner result route redirects to the book page.
- Legacy/full-text analysis does not unlock a new preparation under this
  workflow; the learner reviews a scope and completes a scoped analysis first.
- Rollout prevents new automatic OPDS analysis before the UI advertises the
  learner-initiated Reading Journey Add action and its status model.
- In-flight records are classified deterministically as resumable, completed,
  failed/actionable, or historical; rollout never leaves an indefinite waiting
  state.

Rollout is sequenced as follows:

1. Deploy the explicit scope-review, analysis-status, retry, reconciliation,
   insights, and preparation-prerequisite paths while existing history remains
   readable.
2. Verify that direct per-book OPDS acquisition only stores the source and
   returns to Book detail; it does not create an analysis job. The separate,
   learner-initiated Reading Journey Add action acquires when needed and
   submits whole-book analysis as its explicit backlog behavior.
3. Keep direct catalogue acquisition free of analysis side effects. Existing
   queued work with a live River job is resumable; queued work without one is
   re-enqueued, running work without one becomes failed/actionable, completed
   work remains completed, and legacy records remain historical.
4. Monitor the status and reconciliation paths before enabling new deck
   preparation from the book's current completed scoped analysis only.

## Acceptance criteria

- A learner adds several books from one OPDS feed without navigation and no
  analysis jobs are created by catalogue sync; explicitly adding a book to
  Reading Journey is the learner-initiated exception that submits analysis.
- Confirming a scope creates immutable history but no analysis job.
- Explicit analysis is owner-scoped, asynchronous, idempotent, observable, and
  retryable without orphaned waiting records.
- Metadata-only edits preserve valid scope identity; changed EPUB content
  requires a new review.
- The book page shows at most one current analysis; exact analysis and scope
  identity remain available to backend and operational audit paths.
- Deck preparation rejects anything except an owned completed scoped analysis
  and produces the existing immutable APKG workflow when accepted.
- End-to-end coverage exercises failures, duplicate submissions, retry,
  ownership, source changes, historical results, and rollout compatibility.

## Non-goals

- Automatic analysis from catalogue sync, scope confirmation, or deck
  preparation.
- In-place EPUB content editing.
- Cross-book scopes or aggregate decks.
- Changes to historical classifier data, coverage math, card schema, or
  campaign mastery.
