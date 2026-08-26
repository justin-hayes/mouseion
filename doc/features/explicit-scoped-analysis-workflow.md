# Explicit scoped-analysis workflow

Status: Proposed · Date: 2026-08-26

## Goal

Let learners collect EPUBs, decide exactly what to analyze, inspect completed
results, and prepare a deck through distinct, durable, reproducible steps.

## Learner journey

1. Browse an owner-scoped OPDS catalog.
2. Add an EPUB to My Library without starting analysis or leaving the browser.
3. Open the book, review its extracted units, and confirm a scope revision.
4. Explicitly start asynchronous analysis for that confirmed scope.
5. Observe queued, running, completed, failed, or cancelled status; retry a
   failed or cancelled run when allowed.
6. Inspect insights for a specific completed analysis.
7. Request asynchronous deck preparation from that completed analysis.
8. Observe preparation status and download the immutable ready APKG.

The identity and state contracts for these resources are normative in
[ADR 0028](../adr/0028-explicit-scoped-analysis-lifecycle.md).

## OPDS intake

- Each acquisition control says **Add to library**, not **Import & analyze**.
- A successful add updates that entry in place and preserves the current feed,
  pagination, filters, and scroll-friendly multi-add workflow.
- Repeated acquisition of the same owner, catalog entry, and source content is
  idempotent and reports the existing library book.
- Acquisition downloads and validates the EPUB, persists immutable source
  content and extracted-unit identity, and performs no NLP analysis.
- Partial or failed acquisition is actionable and does not display the book as
  ready for scope review until its source snapshot is valid.

## Scope review and confirmation

- The existing classifier output and selection controls remain review aids.
- Saving a confirmed scope does not start analysis.
- A confirmed scope page identifies its revision, source-content digest/revision,
  unit snapshot, selection summary, and classifier version.
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
  provenance. Insights always identify that analysis.

## Deck preparation prerequisite

- The prepare action is available only from a completed analysis and submits
  that analysis ID, not a mutable book-level analyzed flag.
- The server validates the entire owner-scoped book → source → scope → analysis
  chain and rejects incomplete, failed, stale, legacy-only, or cross-owner input.
- Preparation retry and reconciliation follow ADR 0028 and preserve ADR 0022's
  immutable ready-artifact and pure-download guarantees.
- Deck preparation does not mark vocabulary known. Campaign completion remains
  the explicit graduation boundary in ADR 0027.

## Compatibility and rollout

- Existing completed analyses and prepared decks remain readable and
  downloadable with their historical labels.
- Legacy/full-text analysis does not unlock a new preparation under this
  workflow; the learner reviews a scope and completes a scoped analysis first.
- Rollout prevents new automatic OPDS analysis before the UI advertises the new
  explicit action and status model.
- In-flight records are classified deterministically as resumable, completed,
  failed/actionable, or historical; rollout never leaves an indefinite waiting
  state.

## Acceptance criteria

- A learner adds several books from one OPDS feed without navigation and no
  analysis jobs are created.
- Confirming a scope creates immutable history but no analysis job.
- Explicit analysis is owner-scoped, asynchronous, idempotent, observable, and
  retryable without orphaned waiting records.
- Metadata-only edits preserve valid scope identity; changed EPUB content
  requires a new review.
- Every insight names its completed analysis and exact scope.
- Deck preparation rejects anything except an owned completed scoped analysis
  and produces the existing immutable APKG workflow when accepted.
- End-to-end coverage exercises failures, duplicate submissions, retry,
  ownership, source changes, historical results, and rollout compatibility.

## Non-goals

- Automatic analysis, scope confirmation, or deck preparation.
- In-place EPUB content editing.
- Cross-book scopes or aggregate decks.
- Changes to classifier policy, coverage math, card schema, or campaign mastery.
