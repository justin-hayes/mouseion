# Explicit scoped-analysis workflow

Status: Historical · Date: 2026-08-26 · Updated: 2026-09-09

Issue #691, recorded in [ADR 0054](../adr/0054-retire-standalone-analysis-action.md),
retired the standalone learner-facing analysis action described in this document.
The current trigger is Add to Reading Journey; the details below are retained as
historical context for the analysis contract and compatibility behavior.

> Historical warning: the learner-facing action and submission paths described
> below are not current behavior. Use [ADR 0054](../adr/0054-retire-standalone-analysis-action.md)
> and the canonical workflow documents for current routing and controls.

Shipped collection language is **My Books**. Historical **My Library** / **Add to
My Books** / **Add to library** copy is retained only as a compatibility note;
each catalog entry maps to an owner-scoped Book with active My Books
membership under
[ADR 0035](../adr/0035-my-books-membership-and-source-provenance.md). That
compatibility mapping does not mean every My Books Book has an EPUB. Historically,
a metadata-only Book entered analysis through explicit **Start analysis** or
through the learner's **Add to Reading Journey** intent action; current learners
use only the latter.

## Goal

Let learners collect Books, explicitly start or intentionally trigger current
whole-book analysis, inspect completed results, and prepare a deck through
distinct, durable, reproducible steps.

## Learner journey

1. Connect a learner-owned catalog from the My Books empty state, choose a
   ready catalog language, and synchronize metadata.
2. Browse the local My Books collection and open a metadata-only Book.
3. From Book detail, either explicitly **Start analysis** or add the Book to
   Reading Journey.
4. The selected path acquires the current EPUB when needed and ensures
   whole-book analysis; Journey membership is the learner-initiated automatic
   path.
5. Observe queued, running, completed, failed, cancelled, stale, or unavailable
   status; retry a
   failed or cancelled run when allowed.
6. Inspect the Book's single current analysis on the Book page or Journey
   evidence.
7. Request asynchronous deck preparation there; the request remains bound to
   that completed analysis internally.
8. Observe preparation status and download the immutable ready APKG.

The identity and state contracts for these resources are normative in
[ADR 0028](../adr/0028-explicit-scoped-analysis-lifecycle.md).

## My Books and content acquisition

- Catalog sync creates metadata-only My Books entries; it does not download
  content or start analysis. Book detail's **Start analysis** action acquires and
  analyzes in one explicit flow. **Add to Reading Journey** is the separate
  reading-intent action that performs the same work ensure-once.
- My Books owns active-language-scoped browse, search, paging, and Book
  selection ([ADR 0050](../adr/0050-active-study-language.md)).
- A metadata-only Book offers **Start analysis** and **Add to Reading Journey**
  from its detail page. There is no separate learner-facing acquisition-only
  action in the shipped flow.
- Repeated acquisition of the same owner, catalog entry, and source content is
  idempotent and reports the existing My Books Book.
- **Start analysis** downloads and validates the EPUB, persists immutable source
  content and extracted-unit identity, then submits whole-book analysis.
  **Add to Reading Journey** performs the same work as an ensure-once
  consequence of reading intent.
- Partial or failed acquisition is actionable and does not display current
  evidence until its source snapshot is valid. A Journey member remains in place
  and is shown as unavailable/incomparable when acquisition cannot resolve.

## Analysis input and provenance

- The shipped learner action analyzes the complete current extracted EPUB. There
  is no separate learner-facing scope-confirmation step between acquisition and
  analysis.
- The analysis run remains owner-scoped and records the source content revision,
  extracted-unit snapshot, and immutable result provenance internally.
- Metadata-only title, author, or language edits do not invalidate current
  evidence because they do not replace content.
- When actual EPUB bytes change, the existing result becomes stale. The learner
  must explicitly **Start analysis** again or express Reading Journey intent;
  catalog synchronization never starts a background re-analysis.

## Explicit asynchronous analysis

- **Start analysis** submits the current owner-scoped Book/source for whole-book
  analysis. Adding the Book to Reading Journey submits the same logical request
  as an ensure-once consequence of reading intent.
- Submission returns a durable analysis-run identity and status URL.
- Duplicate submission for the same logical request resolves to the same run;
  concurrent requests do not enqueue competing work.
- Status shows the current state, attempt count, timestamps, and an actionable
  failure without exposing sensitive provider or source data.
- Retry atomically creates or identifies a viable River attempt. A queued or
  running status without viable work is reconciled rather than polled forever.
- Completion publishes one immutable analysis artifact with source and extracted
  unit provenance. A newly completed rerun replaces the Book's current
  learner-facing analysis; earlier artifacts remain operational audit records
  under [ADR 0040](../adr/0040-one-current-analysis-per-book.md).

## Deck preparation prerequisite

- The prepare action is available on the book page only when a current analysis
  is completed. It submits that exact analysis ID internally, not a mutable
  book-level analyzed flag.
- The server validates the entire owner-scoped Book → source → analysis chain
  and rejects incomplete, failed, stale, legacy-only, or cross-owner input.
- Preparation retry and reconciliation follow ADR 0028 and preserve ADR 0022's
  immutable ready-artifact and pure-download guarantees.
- Deck preparation does not mark vocabulary known. Vocabulary graduation uses the single justified transition of [ADR 0036](../adr/0036-primary-goal-justified-graduation.md); reading-finished alone graduates nothing. Campaign operations remain secondary/history only.

## Compatibility and rollout

- Existing completed analyses remain readable as operational audit records, and
  prepared decks remain downloadable with their historical labels. The
  run-specific learner result route redirects to the book page.
- Legacy/full-text analysis does not unlock a new preparation under this
  workflow; the learner must have a current completed analysis first.
- Catalog sync remains metadata-only; the UI advertises the learner-initiated
  Reading Journey Add action and its status model before that action ensures
  analysis.
- In-flight records are classified deterministically as resumable, completed,
  failed/actionable, or historical; rollout never leaves an indefinite waiting
  state.

Rollout is sequenced as follows:

1. Deploy the analysis-status, retry, reconciliation, insights, and
   preparation-prerequisite paths while existing history remains readable.
2. Verify that catalog sync remains metadata-only. Book-detail Start analysis
   and the separate learner-initiated Reading Journey Add action acquire when
   needed and submit whole-book analysis.
3. Keep catalog synchronization free of analysis side effects. Existing
   queued work with a live River job is resumable; queued work without one is
   re-enqueued, running work without one becomes failed/actionable, completed
   work remains completed, and legacy records remain historical. Journey intent
   is the explicit learner action that may ensure analysis.
4. Monitor the status and reconciliation paths before enabling new deck
   preparation from the Book's current completed analysis only.

## Acceptance criteria

- A learner adds several books from one OPDS feed without navigation and no
  analysis jobs are created by catalog sync; explicitly adding a book to
  Reading Journey is the learner-initiated exception that submits analysis.
- Catalog sync creates no analysis job; explicit Start analysis and Journey
  intent are the two learner-facing submission paths.
- Explicit analysis is owner-scoped, asynchronous, idempotent, observable, and
  retryable without orphaned waiting records.
- Metadata-only edits preserve current evidence; changed EPUB content produces a
  stale state that requires explicit re-analysis.
- The book page shows at most one current analysis; exact analysis and scope
  identity remain available to backend and operational audit paths.
- Deck preparation rejects anything except an owned completed current analysis
  and produces the existing immutable APKG workflow when accepted.
- End-to-end coverage exercises failures, duplicate submissions, retry,
  ownership, source changes, historical results, and rollout compatibility.

## Non-goals

- Automatic analysis from catalog sync, background watching, or deck
  preparation.
- In-place EPUB content editing.
- Cross-book scopes or aggregate decks.
- Changes to historical classifier data, coverage math, card schema, or
  campaign mastery.
