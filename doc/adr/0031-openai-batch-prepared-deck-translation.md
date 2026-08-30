# ADR 0031: Execute prepared-deck translation through OpenAI Batch

Status: **Superseded by ADR 0032** · Date: 2026-08-28 · Author: Justin + Hermes

## Context

Mouseion currently translates prepared-deck candidates by calling the OpenAI
Chat Completions endpoint synchronously once per cache miss. Bounded in-memory
concurrency shortens one worker attempt but consumes the synchronous rate-limit
pool and loses unsaved progress when the process exits.

ADR 0030 established durable preparation-owned manifests, item outcomes, retries,
and atomic finalization. Its initial scalar-worker design gives each manifest
item a River job and a leased provider permit. OpenAI Batch instead accepts a
JSONL file of independent requests, executes it asynchronously within a 24-hour
window, and returns unordered output and error files. It has a separate rate
limit and a 50% price discount, but introduces provider jobs that can outlive a
Mouseion worker and cannot be committed atomically with local state.

The product direction is to replace, rather than permanently supplement, the
synchronous prepared-deck translation path. Batch must therefore be part of the
durable run lifecycle, not hidden behind the scalar `TranslationProvider`
interface or treated as a long synchronous call.

## Decision

### 1. Batch is a durable execution layer

Implement OpenAI Batch as a preparation-run execution layer above the scalar
translation request and response semantics. It does not implement the existing
one-call `TranslationProvider` contract for prepared-deck work.

The shared semantic codec owns prompt construction, model parameters, privacy
filtering, Chat Completions request bodies, response decoding, and Mouseion
validation. The synchronous rollout path and Batch transport use that same
codec so transport cannot silently change translation behavior.

This amends ADR 0030's scalar translation dispatch and global provider-permit
design. Its immutable manifest, exact cache identity, per-item terminal outcome,
owner scoping, cancellation, and atomic finalizer remain authoritative. Batch
chunks replace per-item provider jobs and provider permits.

### 2. Batch requires an explicitly capable OpenAI endpoint

The implementation supports the official OpenAI API and
`POST /v1/chat/completions` only. It must not infer Files/Batch support from an
"OpenAI-compatible" base URL or model name. When external prepared-deck
translation is enabled, an ineligible endpoint fails configuration rather than
falling back silently.

The configured model must be frozen into the run and used for every request in
one input file. Unsupported-model or endpoint errors fail the affected Batch
chunk with a bounded, privacy-safe error class. Supporting another endpoint or
Responses API later requires evidence of compatible Files, Batch, request, and
retention behavior and a separate decision.

There is no `sync|batch` mode or temporary Batch feature flag. Prepared-deck
translation uses Batch; synchronous translation remains available only to
other non-prepared-deck enrichment.

### 3. One request represents one manifest item

Each cache miss becomes one JSONL request using the frozen shared request body.
A privacy-safe `custom_id` deterministically encodes the durable run, item
ordinal, and retry generation. It contains no owner ID, source text, lemma,
title, or document metadata. Reconciliation always correlates by `custom_id`;
file order has no meaning.

A chunk contains one model and endpoint. Before upload, Mouseion enforces the
provider's 50,000-request and 200 MB limits and a configurable queued-prompt
budget. The initial request-count ceiling is 5,000, matching expected deck
sizes while leaving operational headroom. Chunks split deterministically by
manifest order before either limit is crossed. Exact serialized byte counts are
calculated; token counts are conservative estimates and the split reason is
recorded.

### 4. Submission is recoverable and fails closed on ambiguity

Before any provider call, Mouseion persists a chunk row containing its local
identity, retry generation, model, endpoint, ordered item set, input digest,
counts, and `submitting` state. The provider Batch metadata includes that opaque
local chunk identity.

After upload and Batch creation, input-file and Batch IDs are persisted on the
same chunk. A crash can occur after the remote Batch is created but before its
ID is committed. OpenAI Batch creation does not provide a transactional boundary
with PostgreSQL, so the reconciler searches recent provider Batches for the
persisted opaque metadata before considering resubmission. If it cannot prove
whether creation occurred, it fails the chunk for operator-visible/manual retry
instead of automatically risking duplicate billable work. Fencing ensures that
an orphaned or duplicate provider result can never overwrite a newer terminal
item outcome.

### 5. Polling and reconciliation use River and persisted state

Short River jobs submit, poll, cancel, and reconcile provider Batches; no River
attempt waits for the 24-hour completion window. Polling uses a 30-second
default interval with bounded jitter and records provider status transitions.
The database, not River metadata, is the source of truth.

For terminal provider states, reconciliation streams output and error JSONL,
validates every line independently, rejects unknown or duplicate IDs, decodes
successful responses through the shared codec, and writes exact immutable cache
rows and item outcomes idempotently. Completed outputs from an expired or
partially failed Batch are retained. Missing, malformed, or contradictory
results are explicit item or chunk failures, never silently treated as success.

Only retryable request failures and expired, uncompleted items enter another
Batch generation. Successful items are never resubmitted. The initial policy
allows two provider Batch generations per item; exhaustion leaves optional
English fields empty and permits deterministic finalization. Orchestration or
identity failures that make outcomes untrustworthy fail the run.

Cancellation changes local run state first, then best-effort cancels each live
provider Batch. Late results from a cancelled or superseded generation are
ignored by fenced writes.

### 6. Provider files are temporary; parsed outcomes are durable

Mouseion persists provider object IDs, bounded status/error classes, counts,
usage, and timestamps, but not uploaded JSONL, downloaded raw output, prompts,
responses, sentences, or raw provider errors as orchestration data. Files are
streamed during upload/download and processed without unrestricted logging.

Input, output, and error files use a seven-day provider expiration as a safety
window and are deleted best-effort after successful reconciliation or terminal
cancellation. Mouseion retains privacy-safe Batch/chunk metadata with the durable
preparation run. File deletion failure is observable but does not invalidate an
already reconciled artifact.

### 7. Prepared decks remain an offline workflow

A learner who opts into external translation accepts that preparation can remain
`preparing` for the provider's 24-hour Batch window and bounded retry lifecycle.
Mouseion exposes phase, age, aggregate progress, and cancellation without adding
a second public preparation state machine. Translation remains optional: after
bounded item failures, the deck may finalize with accurate missing-translation
completeness.

## Alternatives considered

- **Implement Batch behind `TranslationProvider`.** Rejected because a scalar
  request/response interface cannot represent upload, provider job identity,
  polling, partial output, or restart recovery without blocking or hidden state.
- **Add a synchronous/Batch rollout mode.** Rejected because it creates a
  temporary product configuration and two runtime policies. Equivalent request
  and result fixtures provide comparison coverage without a deployment mode.
- **Submit one whole-deck model request.** Rejected because it creates context
  and output-size risk and destroys independent cache, correlation, failure, and
  retry semantics.
- **Automatically resubmit an ambiguously created Batch.** Rejected because
  provider creation is not transactional and duplicate work may be billable.
- **Persist raw provider files locally.** Rejected because parsed cache entries
  and item outcomes are the durable product data; retaining source-derived bulk
  files expands the privacy and deletion surface without aiding finalization.
- **Use only synchronous concurrency.** Rejected as the target architecture
  because it retains the normal rate-limit pool, per-request scheduling, and
  non-durable in-memory lifecycle.

## Consequences

- Prepared-deck translation becomes cheaper and gains a separate provider quota,
  but normal completion latency may be much longer and is not interactive.
- ADR 0030's durable manifest and finalizer remain prerequisites, while its
  scalar item workers and leased provider permits are superseded for the Batch
  path.
- New persisted chunk state and migrations require CODEOWNERS review.
- Ambiguous provider creation fails closed and can require operator/manual retry;
  this favors cost and correctness over automatic liveness.
- Batch is intentionally provider-specific. Generic OpenAI-compatible endpoints
  are not supported for prepared-deck translation after this change.
- Quality, cost, latency, expiry, correlation, and completeness were measured on
  frozen equivalent manifests before this cutover. Batch changes transport, not
  the quality contract.

## Approved operational choices

- Prepared-deck translation support for custom OpenAI-compatible endpoints is
  retired; only the official OpenAI endpoint is eligible.
- Two Batch generations, a 5,000-request ceiling, a 30-second poll interval,
  and seven-day provider-file expiration are the initial defaults.
- Ambiguous remote Batch creation fails closed and requires operator/manual
  recovery rather than risking duplicate billable work.

## Validation and review gate

Issue #353 supplies an explicit operator command and checked-in frozen
fixtures. It compares the same request bodies, model, prompt version, cache
identity, response decoder, and renderer through synchronous and Batch
transports, while local stubs cover success, partial failure, expiry,
cancellation, restart, and ambiguous submission. The report separates those
synthetic results from measured real-provider evidence and records quality,
correctness, privacy, durability, cost, latency, and operational-recovery
gates.

The issue #353 validation gate approved cutover after recording the endpoint,
operational-default, quality, correctness, privacy, durability, cost, latency,
and operational-recovery decisions in the validation workflow. The validation
command is intentionally not a deployed feature flag or runtime transport
selector, and neither CI nor startup makes paid provider calls.

## Related

- [OpenAI Batch API for prepared-deck translation](../features/openai-batch-translation.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0012: External translation through River](0012-enrichment-execution-via-river.md)
- [ADR 0021: Contextual sentence translation cache](0021-contextual-translation-cache.md)
- [ADR 0022: Asynchronous prepared decks](0022-prepared-decks.md)
- [ADR 0030: Durable prepared-deck translation runs](0030-durable-prepared-deck-translation.md)
- [OpenAI Batch API guide](https://developers.openai.com/api/docs/guides/batch)
