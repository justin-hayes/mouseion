# ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts

Status: **Accepted** · Date: 2026-08-26 · Updated: 2026-09-01 · Author: Justin + Codex

## Context

Mouseion currently couples several transitions that need different learner
decisions. OPDS acquisition can enqueue analysis immediately; issue #551 removes learner-facing scope confirmation
can both save a decision and start work, and deck preparation can be offered from
a book-level analyzed flag without naming the completed analysis it consumes.
This makes it difficult to add several books for later review, understand which
scope produced an insight, or reproduce the input to a prepared deck.

The EPUB scope work already defines immutable reviewed selections and historical
corpora. The next workflow must make those concepts durable application
resources and separate acquisition, review, analysis, insight, and preparation
into explicit transitions.

## Decision

Adopt this learner-visible lifecycle:

```text
OPDS browse/add
  → explicit whole-book analysis (internal scope provenance is created automatically)
  → analysis insights
  → deck preparation
  → download
```

Adding a book stores the source and returns the learner to the OPDS browser. It
does not confirm a scope, enqueue analysis, or navigate to an analysis status
page. A learner may add multiple books during one browsing session.

Scope confirmation and analysis submission are separate actions. Confirmation
creates an immutable, owner-scoped scope revision. The learner explicitly starts
analysis for that revision and can observe and retry the asynchronous run.
Preparing a deck requires the identity of a completed analysis and consumes only
that analysis's immutable corpus and scope provenance.

## Durable identity and immutability

### Source content

Each imported EPUB has an immutable source-content revision identified by a
cryptographic digest of the actual EPUB bytes and a versioned extraction
snapshot. Title, author, and language are mutable metadata attached to the book;
editing only those fields does not change content identity or invalidate a
confirmed scope.

Normal application operations do not replace EPUB bytes in place. If a future
operation imports different bytes for the same logical book, it creates a new
source-content revision and extracted-unit snapshot. Existing scopes, analyses,
insights, and prepared decks retain their original source identity. Before the
new content can be analyzed, the learner must review and confirm a scope for its
new snapshot; an old scope is never silently rebound.

### Scope revisions

A confirmed scope is an immutable revision containing its owner, source-content
revision, extracted-unit snapshot, and canonical ordered selected-unit
references. A new confirmed scope's logical identity consists only of those
four values. Editing a selection creates a new scope revision. Existing scopes
remain readable for audit and historical results.

Equivalent confirmation requests are idempotent: the same owner, source-content
revision, extracted-unit snapshot, and canonical ordered selected-unit
references resolve to the same scope revision. References are validated as
readable members of that exact owner-scoped snapshot and serialized in strict
spine order. A different selection or source-content revision creates a new
revision.

Classifier identity and selection mode are not part of scope validation,
serialization, idempotency, the confirmation key, or displayed provenance.
TOC checklist rows are expanded to persisted extracted-unit references before
this contract is applied; a TOC row is not a durable unit identity.

### Retired classifier compatibility decision (superseded)

The earlier implementation retained classifier tables and scope columns as
non-destructive compatibility state and deduplicated equivalent confirmations
through a confirmation key. That compatibility decision was superseded by
[ADR 0039](0039-drop-retired-epub-classifier-schema.md) when migration 000043
removed the retired schema. Shipped migrations 000022 through 000027 remain
immutable history.

### Analysis runs

An analysis run is an immutable request bound to one owner and one confirmed
scope revision. It records analyzer/configuration identity and produces at most
one immutable completed analysis artifact. A retry is a new execution attempt
for the same logical run; it does not rewrite a completed artifact. Reanalysis
with a changed scope, source content, or analysis contract creates a new run.

Completed analyses and their insights remain addressable as history. Book-level
"latest" views may point to a current completed analysis but are not themselves
the identity consumed by downstream work.

### Deck preparations

A deck preparation is bound to one completed analysis artifact and the relevant
owner-scoped learner-state snapshot or version used for candidate selection. A
ready APKG remains an immutable artifact under ADR 0022. Submission and retry
are idempotent for the same preparation request; no queued/preparing record may
be left waiting without a durable River job capable of advancing it.

## Ownership and authorization

Every book, source-content revision, extracted-unit snapshot, scope revision,
analysis run, corpus, insight, preparation, and artifact lookup is owner-scoped.
Browser-provided IDs are references only. The server reloads and validates the
complete chain and rejects missing, stale, cross-owner, or contradictory
relationships before enqueueing work or returning an artifact.

## Status state machines

Scope revision status is intentionally simple:

```text
draft review → confirmed
```

Only `confirmed` revisions can be analyzed. Confirmation is terminal; a change
creates a new draft and revision.

Analysis run status is:

```text
queued → running → completed
             ↘ failed
queued/running → cancelled
failed/cancelled → queued (explicit retry attempt)
```

Deck preparation retains ADR 0022's states:

```text
queued → preparing → ready
                    ↘ failed
queued/preparing ─────→ cancelled
failed/cancelled → queued (explicit retry attempt)
```

`completed` analysis artifacts and `ready` deck artifacts are terminal and
immutable. Retrying is allowed only from a retryable terminal state and must
atomically establish both the durable domain transition and its corresponding
River job.

## Failure, retry, and reconciliation

- Creation of a queued analysis or preparation and enqueueing its River job
  occur in one database transaction.
- A retry either reuses the existing live job or atomically creates a new
  attempt; it cannot only change UI/domain status.
- Duplicate submissions return the existing logical resource and live job
  rather than creating competing work.
- Worker attempts validate ownership and prerequisite identities again before
  processing, and make completion atomic with artifact persistence.
- Failures preserve their last error and attempt history. Learners can retry
  retryable failures explicitly; validation failures require a new valid scope
  or request.
- A reconciliation path detects queued/running records with no viable job and
  marks them failed or safely re-enqueues them according to the recorded
  idempotency key. The UI must not poll forever without an actionable state.
- A failed or cancelled analysis produces no completed analysis artifact. A
  failed or cancelled preparation does not reserve vocabulary permanently.

## Consequences

- OPDS browsing becomes an intake workflow rather than an analysis trigger.
- Learners can build a library first and review books independently.
- Insights and prepared decks can name the exact analysis and scope they use.
- Persistence needs explicit source, scope, run, attempt, and artifact
  identities plus constraints that enforce their relationships.
- Existing legacy/full-text analyses remain readable but cannot satisfy the new
  deck-preparation prerequisite until a confirmed scope analysis completes.
- Migration and rollout must distinguish existing immutable history from
  resumable in-flight work and must not infer mastery from generated decks.

## Non-goals

- Editing or replacing EPUB content in normal operation.
- Reusing classifier or recommendation behavior in new scope review.
- Automatically analyzing an OPDS acquisition.
- Mutating a confirmed scope or completed analysis in place.
- Combining scopes or analyses across books.
- Changing coverage thresholds, classifier heuristics, NLP contracts, Anki card
  fields, campaign semantics, or the known/generated vocabulary distinction.
- Adding object storage, live push updates, or a general workflow engine.

## Related decisions and specifications

- [ADR 0010: Adopt River as the background-job queue](0010-river-job-queue.md)
- [ADR 0022: Asynchronous deck preparation and durable APKG artifacts](0022-prepared-decks.md)
- [ADR 0025: Analysis coverage and threshold metric contract](0025-analysis-coverage-threshold-metrics.md)
- [Explicit scoped-analysis workflow](../features/explicit-scoped-analysis-workflow.md)
- [EPUB Analysis Scope Review](../features/epub-analysis-scope-review.md)
- [Analysis Insights](../features/analysis-insights.md)
