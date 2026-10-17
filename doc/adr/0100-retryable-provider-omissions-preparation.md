# ADR 0100: Retryable per-item omissions for prepared decks

Status: **Accepted** · Date: 2026-10-17 · Author: Justin + opencode

Tracked by [#2126](https://github.com/justin-hayes/mouseion/issues/2126). Amends
the standard-run completeness rule in [ADR 0032](0032-standard-first-prepared-deck-translation.md)
§6 and clarifies the per-item outcome model in [ADR 0030](0030-durable-prepared-deck-translation.md)
§3.

## Context

ADR 0032 §6 made a consented standard run fail closed: every accepted manifest
item that required external translation had to have an exact cached result
before the run could publish. That is correct for a transport, authentication,
configuration, or cancellation failure, because the run cannot trust its own
work. It is too strict for a *response* failure: a provider that returns an
unusable answer for one item, even after the bounded retry budget, is a
per-item fact, not a reason to discard the hundreds of items that succeeded.

A real Ancient Greek preparation demonstrated the cost: a run failed with 390 of
397 and then 393 of 397 items translated because a handful of answers could not
be validated. Those identities are exactly the ones a learner would want to
retry, and re-requesting the other hundreds is wasteful when their immutable
cache rows already exist.

## Decision

### 1. A per-item provider response failure is a retryable omission

When a standard run's items are all terminal, a failed item whose bounded error
is a provider response failure is tolerated. The run finalizes and the
preparation becomes `ready` with the cards that did succeed. The failed
identities are reported to the learner as omissions with a fixed, learner-facing
reason: **"N words left out: the translation provider gave an unusable
answer."** The tolerated classes are the response-validation classes only:
`invalid_response`, `invalid_translation_response`, `malformed_response`, and
`response_too_large`. They are enumerated once and shared by the run-advance
predicate and the finalizer's stored-result derivation.

### 2. Every other failure class still fails the run

Transport and timeout failures, rate limits exhausted, HTTP 401/403, invalid
requests and endpoints, provider unavailability, cache and persistence faults,
cancellation, and identity or orchestration faults are not response failures
and continue to fail the run exactly as before. A run in which **no** accepted
item produced a usable result is also still failed, even if every failure was a
response failure: a deck with zero cards is not a usable artifact, and this
preserves the existing "all meanings unresolved" outcome.

### 3. An omitted item is not generated, reserved, or silently dropped

A tolerated omission is rendered as an explicit meaning omission. It is
therefore excluded from the artifact's cards, so it is never written to
generated vocabulary (`generated_vocabulary`) or attached to the preparation's
vocabulary (`deck_preparation_vocabulary`). A later preparation of the same
frozen Reading snapshot is not blocked by it; the successful identities are
cache hits and only the omission is re-requested. Preparation still never writes
Known vocabulary and never mutates the frozen Reading snapshot. The ready status
projection reports the omission count and reason on both the Reading page and
the deck page.

### 4. Retrying the omissions reuses every other cached result

The learner retries from the ready deck's retry action. It re-freezes the same
Current-reading snapshot (the existing re-preparation path, [ADR 0076](0076-reprepare-ready-deck.md)),
so the exact cache identity of every successful item is unchanged and resolves
from the immutable enrichment cache without a provider call. Only the previously
omitted identities miss and reach the provider. The recovered cards join the new
generation's deck.

### 5. Batch keeps its existing offline partial-result policy

The standard interactive run is the learner-facing path this decision tightens.
Explicit offline Batch runs retain the partial-result and finalization policy of
ADR 0031; this ADR does not change Batch's run-advance behavior. Batch item
outcomes that are response failures can still be understood as omissions by the
shared stored-result derivation, so the two modes do not disagree about what a
failed item means.

## Consequences

- A preparation is more likely to finish with a usable deck instead of failing
  wholesale, at the cost of an artifact that may omit a few selected identities.
- Learners see an honest count and reason and have a cheap retry that does not
  re-pay for the successful translations.
- The set of tolerated error classes is a bounded, reviewable list; widening it
  is an explicit change to this ADR.
- The finalizer must derive an omission for a tolerated failed item rather than
  treating a missing required result as an invalid manifest, and the run-advance
  predicate must count non-tolerable failures. Both are covered by integration
  tests.
- A run where every item fails, and any run with a non-response failure, remains
  a failed preparation with no artifact.

## Alternatives considered

- **Keep failing the whole run on any failed item.** Rejected: it discards
  already-paid, already-cached successful work and asks the learner to retry
  everything to recover a few items.
- **Silently drop failed items without a learner-facing omission.** Rejected:
  the learner would not know selected vocabulary is missing and could not retry
  it.
- **Retry only the omitted items inside the same run.** Rejected: the run,
  manifest, outcomes, and published artifact are immutable and atomically
  published. Reusing the existing new-generation retry path keeps that
  invariant while still reusing the cache and re-requesting only the omissions.
- **Tolerate all failed classes.** Rejected: it would publish a deck after a
  transport, authentication, configuration, or cancellation failure the run
  cannot trust.
- **Change Batch's partial-result policy at the same time.** Deferred: Batch is
  operator-selected offline work with its own accepted policy; this decision is
  about the interactive path.

## Related

- [ADR 0030: Durable prepared-deck translation runs](0030-durable-prepared-deck-translation.md)
- [ADR 0031: Execute prepared-deck translation through OpenAI Batch](0031-openai-batch-prepared-deck-translation.md)
- [ADR 0032: Make standard requests the interactive prepared-deck translation default](0032-standard-first-prepared-deck-translation.md)
- [ADR 0076: Roll forward when a ready deck requires re-preparation](0076-reprepare-ready-deck.md)
- [Fast user-facing prepared-deck translation](../features/fast-user-facing-translation.md)
