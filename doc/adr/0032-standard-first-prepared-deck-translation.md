# ADR 0032: Make standard requests the interactive prepared-deck translation default

Status: **Accepted** · Date: 2026-08-29 · Author: Justin + Hermes

## Context

ADR 0031 made OpenAI Batch the only prepared-deck translation transport. That
choice preserved durable manifests, exact item outcomes, restart recovery, and
atomic APKG finalization while reducing cost and synchronous rate-limit pressure.
It also accepted OpenAI's asynchronous 24-hour completion window.

Real prepared-deck runs have taken hours. A learner who explicitly requests an
English-translated deck expects the downloadable artifact within an interactive,
predictable preparation workflow, not an offline processing window. Batch remains
useful for cache prewarming and other delayed work, but its latency contract does
not fit the primary learner-facing path.

The implementation now contains the durable scalar foundation and standard
worker described here, including the foundation that ADR 0030 introduced
before ADR 0031 superseded its dispatch path:

- `deck_preparation_runs`, immutable manifests, and per-item
  `deck_preparation_translation_outcomes`;
- item claim tokens, leases, attempt counts, `next_attempt_at`, generations, and
  cache/provider latency counters;
- exact immutable enrichment-cache reads and writes;
- a recovery projection that can discover pending or expired scalar outcomes;
- an idempotent finalizer and atomic APKG completion boundary.

The change therefore does not require an in-memory workflow or a general-purpose
executor framework. It requires an explicit run execution mode, a standard
item-worker implementation, bounded provider capacity, and honest status and
telemetry.

## Decision

### 1. Interactive runs use standard execution

Prepared-deck translation runs freeze an execution mode of `standard` or `batch`.
`standard` is the default selected for the existing learner-facing preparation
request. It uses the configured OpenAI-compatible Chat Completions client because
that is Mouseion's current provider compatibility contract.

`batch` remains an explicit, non-default mode for operator-selected offline or
economy work. The initial change does not add a learner-visible economy selector.
Changing process configuration must not change an already-frozen run's mode.
There is no automatic standard-to-Batch fallback after timeout or retry exhaustion.

This accepted ADR supersedes ADR 0031 sections 2 and 7 only where
they require Batch as the sole prepared-deck transport and define prepared decks
as an offline workflow. ADR 0031's Batch submission, correlation, reconciliation,
cancellation, cleanup, and file-retention decisions continue to govern `batch`
runs. ADR 0030's durable manifest, item outcomes, recovery, and atomic finalizer
remain authoritative for both modes.

### 2. Share domain semantics, not provider lifecycle objects

The execution boundary is a narrow preparation-run planner/dispatcher selected by
the frozen mode. It consumes the same immutable manifest and creates durable work:

- `standard` creates or dispatches one River item job per eligible cache miss;
- `batch` creates the existing durable chunks and submission jobs.

Both modes converge on the same per-item outcome states, immutable cache rows, and
finalizer. Batch polling is not forced into the scalar `TranslationProvider`
interface, and standard calls are not represented as synthetic Batch objects.
OpenAI transport types remain in `internal/enrichment` and mode-specific workers.

### 3. One standard request represents one manifest item

The initial standard executor sends one manifest item per request. This deliberately
avoids grouping until latency, token, and rate-limit telemetry justify it. The
stable work identity is the run ID, manifest ordinal, candidate digest, and dispatch
generation. Strict structured output carries an opaque item ID and must match the
requested identity before a result can be cached or marked complete.

A schema/prompt change increments the translation codec version. Both standard and
Batch use the same request builder and response decoder so execution mode cannot
change translation semantics.

### 4. Concurrency is bounded by a dedicated River queue

Standard item jobs use a dedicated `prepared_deck_translation` River queue. Its
`MaxWorkers` is configured by
`MOUSEION_PREPARED_DECK_STANDARD_MAX_CONCURRENCY`, initially `4`. Each item job
performs at most one provider attempt. The queue therefore provides a hard
per-process bound on simultaneous standard provider calls without launching an
unbounded goroutine per card.

Mouseion currently runs one worker process. If it is horizontally replicated,
the deployment-wide maximum is the configured value multiplied by the number of
worker processes; deployment documentation must state that constraint. A future
global provider-permit lease is required before independent replicas may claim a
single account-wide bound.

The request timeout continues to use `MOUSEION_LLM_TIMEOUT`. Standard prepared-deck
retry settings are:

- `MOUSEION_PREPARED_DECK_STANDARD_MAX_ATTEMPTS`, default `3`;
- `MOUSEION_PREPARED_DECK_STANDARD_RETRY_BASE_DELAY`, default `1s`;
- `MOUSEION_PREPARED_DECK_STANDARD_RETRY_MAX_DELAY`, default `30s`.

Retries use exponential backoff with bounded jitter. Delay is persisted through
`next_attempt_at` and scheduled River work; a worker does not sleep while holding
an item claim. HTTP 408, 429, 5xx, connection failures, and request timeouts are
retryable. Cancellation, invalid configuration, identity mismatches, and invalid
structured output are terminal unless the error is explicitly classified as a
retryable provider response failure within the bounded attempt policy.

### 5. Progress and recovery remain item-durable

A successful standard response and its exact immutable cache row are committed
before the item becomes `completed`. A restart recovers pending work and expired
claims from persisted outcomes. Completed outcomes are never submitted again.
Stale generations cannot write after cancellation, retry, or supersession.

The public preparation state machine remains `queued`, `preparing`, `ready`,
`failed`, and `cancelled`. During `preparing`, standard runs report a
`translating` phase and aggregate completed, running, retrying, pending, and failed
counts. Batch-only fields and language such as “Waiting for Batch translation” are
shown only for Batch runs.

### 6. Translated interactive decks fail closed on incomplete results

For a consented interactive `standard` run, every accepted manifest item requiring
external translation must have a successful exact cache result before finalization.
Retry exhaustion fails the run and preparation with a bounded, user-safe error. It
must not publish an APKG with silently missing requested translations.

Runs without consent preserve the existing local-only behavior. Existing Batch
partial-result policy remains scoped to explicit offline Batch runs until a later
product decision changes it. Download remains a pure owner-scoped read of a ready,
immutable artifact.

### 7. Cache identity names the target language

The shared enrichment cache remains cross-user because its key contains no owner or
document identity and identical semantic requests may safely reuse immutable
results. Its versioned key explicitly includes:

- source language, canonical lemma, UPOS, tested source sentence hash, and relevant
  context mode;
- target language (`en` for the current product);
- provider, model, prompt/schema version, and response semantics.

The current provider version already combines model and prompt version, and the
sentence hash binds contextual source text. A migration adds explicit target
language to the cache and frozen run/item identity so future target-language
support cannot collide with existing English rows. Existing rows backfill to `en`.

### 8. Measure before setting a latency SLO

The initial product objective is “a few minutes for a representative deck,” not a
hard SLO. Standard execution records low-cardinality metrics or structured logs for
run duration, item and request counts, request latency, retries, rate limits,
validation failures, cache hits/misses, token usage, execution mode, and outcome.
No exact completion estimate is shown until representative production telemetry
exists.

A paid, explicitly acknowledged manual smoke test records real endpoint latency,
cost/usage, correctness, APKG importability, and comparison with the prior Batch
report. Fake-provider tests prove contracts and failure behavior, not real OpenAI
throughput.

## Product choices resolved for the initial implementation

- No learner-visible economy mode; Batch is configuration/operator selected.
- No untranslated early download while translation continues.
- One manifest item is the durable and request unit; no grouped standard requests.
- English (`en`) is the explicit initial target language.
- The existing configured model remains the standard model; model-quality changes
  are a separate decision.
- Requested standard translation fails clearly after bounded exhaustion instead of
  publishing a partial translated artifact.
- The semantic cache remains shared across users.
- Latency is measured before an SLO or ETA is promised.

## Alternatives considered

- **Keep Batch as the interactive default.** Rejected because observed hours-long
  completion and a 24-hour provider window do not meet the learner-facing workflow.
- **Use Flex processing.** Rejected because it is a lower-priority service tier,
  can be resource-constrained, and requires long request timeouts.
- **Use Fast/Priority processing immediately.** Deferred until standard concurrency
  supplies measured latency and cost evidence.
- **One long-running worker with an in-memory goroutine pool.** Rejected because it
  weakens item-level restart recovery and duplicates scheduling already represented
  by River and persisted outcomes.
- **Force both transports behind a scalar submit/poll interface.** Rejected because
  Batch has durable provider jobs and files while standard requests have immediate
  responses; a common lifecycle interface would leak or hide critical semantics.
- **Group many items in one standard request.** Deferred because it increases
  validation complexity and failure blast radius before basic concurrency is
  measured.
- **Automatically fall back to Batch.** Rejected because timeouts are ambiguous,
  completed remote work may still be billable, and Mouseion has no cross-transport
  reconciliation contract.
- **Keep target language implicit in the prompt version.** Rejected because target
  language is a material semantic input and must be independently inspectable.

## Consequences

- Interactive translation uses the normal provider rate-limit pool and costs more
  than Batch, offset by exact cache reuse and bounded concurrency.
- The accepted Batch-only architecture becomes dual-mode at the execution layer,
  while sharing durable domain state and finalization.
- A database migration is required for execution mode and target-language identity;
  normal Mouseion CODEOWNER/manual merge rules apply.
- Strict translated-deck completeness may fail some jobs that previously produced
  artifacts with missing English fields; this is intentional and user-visible.
- Standard worker capacity must be tuned from telemetry and deployment replica
  count rather than treated as a permanent default.
- Batch remains maintained and tested for explicit offline work.

## Open questions before acceptance

None block the initial implementation choices above. Product review may change the
initial concurrency default, but it remains configuration rather than architecture.
A learner-visible economy mode, additional target languages, grouping, and a hard
latency SLO require later evidence and decisions.

## Related

- [Fast user-facing prepared-deck translation](../features/fast-user-facing-translation.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0010: River job queue](0010-river-job-queue.md)
- [ADR 0012: External translation through River](0012-enrichment-execution-via-river.md)
- [ADR 0021: Contextual translation cache](0021-contextual-translation-cache.md)
- [ADR 0022: Asynchronous prepared decks](0022-prepared-decks.md)
- [ADR 0030: Durable prepared-deck translation runs](0030-durable-prepared-deck-translation.md)
- [ADR 0031: Execute prepared-deck translation through OpenAI Batch](0031-openai-batch-prepared-deck-translation.md)
