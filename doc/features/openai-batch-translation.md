# OpenAI Batch API for prepared-deck translation

Status: **Proposed research** · Date: 2026-08-27

## Summary

Evaluate an explicit OpenAI Batch API execution mode for the external
translation and card-enrichment requests used during prepared-deck generation.
Batch is technically compatible with the current OpenAI integration because
Mouseion sends Chat Completions requests, and OpenAI Batch supports
`POST /v1/chat/completions`. It is a throughput and cost optimization, not a
translation-quality improvement.

The recommended direction is to replace synchronous provider calls with Batch
for prepared-deck translation. A normal preparation should submit one Batch
job containing one request per eligible translation item. Batch should
integrate with the durable prepared-deck translation-run design rather than be
hidden inside the current in-memory worker loop. The current synchronous
provider and its per-deck concurrency setting should be removed after the
Batch path is operationally validated; this is not intended to become a
permanent dual-provider architecture.

## Evidence from the current implementation

The current path has the following boundaries:

- `internal/enrichment/llm.go:OpenAITranslationClient.Translate` constructs and
  sends a synchronous `POST /chat/completions` request using the configured
  model, prompt, JSON response format, temperature or reasoning effort, and a
  30-second default HTTP timeout.
- `internal/enrichment/enrichment.go:Service.EnrichExternal` performs the
  immutable cache lookup, calls the `TranslationProvider`, applies bounded
  retry/backoff policy, validates the translation response, and stores cache
  results.
- `internal/prepareddeck/jobs.go:Worker.Work` builds a manifest, enriches all
  eligible candidates, renders one artifact, and commits the prepared deck as
  one atomic completion.
- `internal/prepareddeck/jobs.go:enrichCandidates` creates a fixed-size
  in-memory worker pool. It assigns results to candidate indexes, preserving
  deterministic output order while allowing up to the configured number of
  provider calls in flight.
- `internal/prepareddeck/config.go` reads
  `MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY`, requiring a positive integer
  and defaulting to `1` for serial compatibility.
- `cmd/server/main.go` wires that value into
  `prepareddeck.AddWorkerWithTranslationConcurrency`.
- The River `prepared_decks` queue remains configured with one whole-deck worker;
  the environment variable changes only the per-deck in-flight enrichment
  calls, not the number of decks prepared simultaneously.
- `internal/prepareddeck/concurrency_test.go` verifies bounded concurrency,
  deterministic serial/concurrent artifact equivalence, non-fatal per-item
  failures, and cancellation behavior.
- `doc/adr/0030-durable-prepared-deck-translation.md` proposes durable,
  preparation-owned translation runs with immutable manifests, per-item
  outcomes, provider leases, fenced claims, and idempotent finalization. Its
  current decision does not specify a Batch API provider.

The current external provider boundary deliberately accepts only language,
canonical lemma, UPOS, tested surface form, and an optional complete example
sentence. A Batch implementation must preserve that privacy boundary. It must
not put owner IDs, document metadata, or prompts/responses into durable River
arguments or unrestricted logs.

## OpenAI Batch API findings

According to the [OpenAI Batch API guide](https://developers.openai.com/api/docs/guides/batch.md):

- Batch accepts JSONL input containing one request per line.
- Supported endpoints include `/v1/responses` and `/v1/chat/completions`.
- Every request requires a unique `custom_id`.
- Each input file can contain requests for only one model.
- A batch may contain up to 50,000 requests and an input file may be up to
  200 MB.
- Batch creation is subject to a separate limit of up to 2,000 batches per hour.
- Batch processing uses a separate rate-limit pool from synchronous requests.
- OpenAI advertises a 50% cost discount relative to synchronous API usage.
- Processing is asynchronous with a `24h` completion window; it may complete
  sooner but cannot be treated as an interactive low-latency API.
- Results are downloaded from output and error files through the Files API.
- Output order is not guaranteed. Results must be correlated by `custom_id`.
- A batch may partially succeed. Completed responses remain available when
  other requests expire or fail.
- Expired requests are reported in the error file and completed requests remain
  billable.
- Batch output files are automatically deleted 30 days after completion.
- Batch support is model-dependent and must be validated for the configured
  model.

## Problem and motivation

The concurrency setting improves wall-clock time for a single prepared deck but
leaves Mouseion responsible for scheduling every request, handling retry bursts,
and consuming the normal synchronous rate-limit pool. It also does not make
translation progress durable: the current worker keeps the manifest and all
candidate outcomes in memory until final rendering.

Batch could simplify high-volume submission, reduce cost, and use additional
provider capacity. It does not explain or directly fix lackluster results. Poor
results may instead arise from prompt construction, model choice, context
selection, response parsing, target alignment, cache identity, or quality
policy. Batch and synchronous modes must therefore use the same request builder
and quality validation so transport changes can be evaluated independently.

## Goals

1. Support large, non-interactive prepared-deck translation runs through OpenAI
   Batch API.
2. Reduce synchronous rate-limit pressure and API cost where Batch is
   appropriate.
3. Preserve exact correlation between source candidates and responses.
4. Handle partial success, expiration, cancellation, retries, and worker
   restarts safely.
5. Replace synchronous per-item scheduling and its concurrency configuration
   rather than carrying both implementations indefinitely.
6. Measure translation quality separately from throughput, cost, and provider
   errors.

## Non-goals

This feature does not promise to:

- improve model quality or prompt quality;
- make small jobs complete immediately or within a synchronous request latency;
- provide immediate responses;
- eliminate retries or malformed model output;
- change candidate selection, sentence selection, or card-ranking policy;
- replace the durable translation-run architecture;
- support arbitrary OpenAI-compatible servers that do not implement Files and
  Batch APIs;
- rely on output line order.

## Proposed product behavior

Use Batch as the prepared-deck translation execution model. During rollout,
an explicit deployment flag may be useful, but it should select a temporary
rollout phase rather than establish a permanent `sync|batch` provider API.
The target configuration is Batch-only. The existing setting

```text
MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY
```

should be deprecated and removed with the synchronous implementation. It must
not be repurposed as a Batch parallelism control: OpenAI schedules requests in
the submitted Batch.

The only required Batch-specific setting should initially be the existing LLM
configuration plus any polling/operational defaults. Possible settings include:

```text
MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS
MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL
MOUSEION_PREPARED_DECK_BATCH_MAX_WAIT
```

Exact names and defaults require an implementation ADR and should follow the
existing configuration patterns.

## Proposed architecture

### Request construction

Extract or reuse the current Chat Completions request construction so the Batch
request uses the existing prompt, model parameters, JSON response format, and
privacy filtering. Serialize one JSONL request per translation unit. Do not
serialize the entire deck as one model request: that would create context and
output-size risk and would eliminate independent item failure and retry
semantics.

Each request needs a stable ID, for example:

```json
{
  "custom_id": "prepared-deck:{run_id}:{item_id}:translation",
  "method": "POST",
  "url": "/v1/chat/completions",
  "body": {
    "model": "configured-model",
    "messages": [],
    "response_format": {"type": "json_object"}
  }
}
```

The example is illustrative; the body must be generated from the existing
request contract. `custom_id` must be unique within the batch, deterministic,
privacy-safe, and independent of JSONL order. It should identify a durable run
and manifest item without embedding source sentences or learner identity.

### Submission

1. Read the immutable translation-run manifest.
2. Exclude cache hits and already-terminal items according to the run contract.
3. Group requests by endpoint and model; never mix models in one input file.
4. For the expected 3,000–5,000-card decks, submit one Batch containing all
   eligible per-item requests when the measured JSONL size and queued prompt
   token budget allow it.
5. Split into multiple Batch jobs only when the 50,000-request limit, 200 MB
   input-file limit, or model/account queued-prompt-token limit requires it.
6. Upload each JSONL file through the Files API with `purpose="batch"`.
7. Create a Batch with the supported endpoint and `completion_window="24h"`.
8. Persist the batch ID, input file ID, model, endpoint, request count, and
   submission state before returning success.

The 50,000-request and 200 MB limits are necessary but not sufficient sizing
checks. The implementation must calculate serialized bytes and estimated
prompt tokens before submission and expose the reason when a run is split.

The current in-memory `Worker.Work` flow is not a safe place to own this
lifecycle: it renders and commits in one River attempt and has no durable place
to resume a provider job that outlives the worker.

### Polling and reconciliation

A durable River job should poll or reconcile Batch status. On a terminal state:

1. Download the output file, if present.
2. Download the error file, if present.
3. Parse each JSONL line independently.
4. Resolve lines by `custom_id`, never by line position.
5. Run the same response decoding and quality validation currently used by the
   synchronous provider, after extracting that logic from the synchronous
   transport implementation.
6. Persist successful item outcomes idempotently.
7. Persist request-level errors separately from batch-level failures.
8. Leave retryable or expired items eligible for a bounded retry policy.
9. Finalize the prepared deck only after all run items are terminal and the
   existing atomic artifact boundary is safe.

Reconciliation must tolerate unordered output, duplicate polling, malformed
lines, unknown IDs, missing output/error files, partial success, expiration,
and worker restart.

### State and idempotency

The exact schema belongs in a follow-up ADR, but durable state must represent:

- preparation and translation-run identity;
- Batch ID and input/output/error file IDs;
- endpoint and model;
- request/chunk count;
- lifecycle status and timestamps;
- per-item custom-ID mapping or deterministic reconstruction;
- reconciliation checkpoint/idempotency information;
- batch-level and request-level error classification.

A worker restart must resume polling or reconciliation rather than submit a
second Batch for the same run. Successful items must not be regenerated merely
because another item failed.

## Failure and retry policy

Distinguish provider orchestration failures from content failures.

### Batch-level failures

Examples include invalid JSONL, upload failure, unsupported model, batch creation
failure, cancellation, or expiration. These affect orchestration and should be
recorded on the run without discarding successful item outcomes.

### Request-level failures

Examples include invalid request parameters, transient provider errors, refusals,
malformed JSON, or content failing Mouseion validation. Only explicitly
retryable classes should be retried. Retries must be idempotent at the
manifest-item level and must not overwrite a newer successful outcome.

Retries should remain Batch-based so the replacement does not carry a second
synchronous transport. A retry run may submit a new Batch containing only the
failed or expired items. This keeps the provider surface singular while
avoiding regeneration of successful items.

## Quality and performance evaluation

Before removing the synchronous implementation, compare equivalent synchronous
and Batch runs using the same frozen manifests, prompt version, model, and
validation rules. Record:

- completion latency and queue latency;
- total and per-item cost;
- cache-hit ratio;
- provider calls, retries, and rate-limit errors;
- parse and validation failure rates;
- partial and expired request counts;
- translation and target-alignment quality;
- card completeness and omission counts;
- duplicate or incorrectly correlated outcomes.

A Batch result that is cheaper or faster but has worse translation or target
alignment is not an acceptable optimization without an explicit product decision.

## Acceptance criteria

### Functional

- Batch is the prepared-deck translation execution model after rollout.
- The synchronous transport and `MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY`
  are removed once migration verification passes.
- Batch requests use the same semantic request contract as the current sync
  requests.
- Every request has a deterministic unique `custom_id`.
- Model and endpoint compatibility is validated before submission.
- Request-count and file-size limits are enforced before upload.
- Batch and file IDs are persisted durably.
- Results are correlated by ID and are independent of output order.
- Mixed success/error output is reconciled correctly.
- Expired batches retain completed item results.
- Failed items are retryable without duplicating successful items.
- Reconciliation is safe across worker restarts and repeated execution.
- Final APKG rendering remains ordered, deterministic, and atomic.

### Tests

Add tests for deterministic JSONL serialization, custom-ID round trips,
output-order independence, mixed success/error files, malformed lines, unknown
IDs, duplicate IDs, batch expiration, cancellation, partial success, restart
recovery, retry selection, chunking, model validation, and migration away from
the current synchronous concurrency behavior.

### Operations

Expose privacy-safe aggregate metrics for submitted batches, batch age, state
transitions, request counts, completed/failed/expired items, reconciliation
errors, retries, cost, and quality-validation failures. Do not log prompts,
responses, source sentences, credentials, or raw provider error bodies.

## Architectural decisions required before implementation

1. Whether Batch is a provider implementation under the current
   `TranslationProvider` boundary or a separate durable translation-run
   execution layer.
2. Whether the durable translation-run design in ADR 0030 is accepted before
   Batch work begins. Batch should not be bolted onto the current in-memory
   loop as a hidden asynchronous request.
3. Which configured models and endpoints are eligible for Batch.
4. Whether failed Batch items retry in a new Batch immediately or through a
   subsequent durable retry run.
5. How long Batch metadata and temporary file IDs are retained.
6. Whether a prepared deck waits up to 24 hours, or whether Batch is restricted
   to an explicitly offline preparation workflow.
7. Whether the API key and uploaded source-derived content are acceptable under
   the existing external-translation privacy policy.

## Suggested implementation decomposition

1. **Provider contract and request serialization:** extract the existing
   request semantics from the synchronous transport, make them the Batch
   request contract, and add deterministic JSONL fixtures.
2. **Batch client and configuration:** implement Files/Batch submission,
   status retrieval, cancellation, and model/endpoint validation behind a
   testable interface.
3. **Durable run integration:** extend the ADR 0030 translation-run lifecycle
   with Batch chunk state, durable IDs, polling, and fenced reconciliation.
4. **Retry policy:** implement per-item retry classification and new Batch
   submissions for failed or expired items; remove synchronous fallback.
5. **Observability and rollout:** add aggregate metrics, operational controls,
   comparison tests, and an opt-in deployment path.

## Related documents and code

- [Durable prepared-deck translation](durable-prepared-deck-translation.md)
- [Prepared-deck LLM translation performance](llm-translation-performance.md)
- [ADR 0030: Durable prepared-deck translation runs](../adr/0030-durable-prepared-deck-translation.md)
- [ADR 0021: Contextual translation cache and privacy](../adr/0021-contextual-translation-cache.md)
- `internal/enrichment/llm.go:OpenAITranslationClient.Translate`
- `internal/enrichment/enrichment.go:Service.EnrichExternal`
- `internal/prepareddeck/jobs.go:Worker.Work`
- `internal/prepareddeck/jobs.go:enrichCandidates`
- `internal/prepareddeck/config.go:TranslationConcurrencyFromEnv`
- `cmd/server/main.go`
- `internal/prepareddeck/concurrency_test.go`
- [OpenAI Batch API guide](https://developers.openai.com/api/docs/guides/batch.md)
