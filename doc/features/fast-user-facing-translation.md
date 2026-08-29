# Fast user-facing prepared-deck translation

Status: **Proposed** · Date: 2026-08-29 · Owner: Mouseion

## Summary

Make bounded standard OpenAI-compatible Chat Completions requests the default
translation execution strategy for the existing learner-facing prepared-deck
workflow. Preserve Mouseion's asynchronous River architecture, immutable manifest,
item-durable progress, exact shared cache, atomic APKG finalization, and pure
download contract. Retain OpenAI Batch only as an explicit non-default executor
for offline work.

This feature reverses the learner-facing transport choice in accepted ADR 0031,
which intentionally optimized for Batch throughput and cost despite its 24-hour
window. It does not reverse the durable run architecture in ADR 0030 or the Batch
implementation itself.

## Problem

Prepared-deck translation through OpenAI Batch has taken hours in real testing.
That is valid Batch behavior but does not match a learner who selects English
translation and waits to download a generated `.apkg`.

The present system already exposes useful durable state, but every consented
prepared-deck run creates Batch chunks. The status UI tells users it is submitting
or waiting for Batch, the worker polls provider jobs, and retry exhaustion may
still permit a deck with missing English fields. The product needs a fast default
without returning to one long, in-memory preparation attempt.

## Goals

1. Use standard Chat Completions requests for interactive deck preparation by
   default.
2. Bound request concurrency and every provider attempt.
3. Persist each successful item before the complete deck finishes.
4. Resume pending work after worker restart without repeating completed items.
5. Reject malformed, incomplete, duplicate, unexpected, or mis-correlated output.
6. Reuse exact versioned translations, including explicit target-language identity.
7. Report honest item progress and terminal failures.
8. Build and expose the APKG only after all requested standard translations exist.
9. Retain the tested Batch lifecycle for explicit offline work.
10. Produce latency, cost, retry, rate-limit, cache, and outcome telemetry that can
    guide later tuning.

## Non-goals

- Prompt-quality or translation-policy redesign beyond the item-ID, target-language,
  and schema-version changes required for strict output.
- A new provider or a general workflow engine.
- A learner-facing economy toggle in the initial release.
- Automatic standard-to-Batch fallback.
- An untranslated early download followed by artifact replacement.
- Grouping several translation items in one standard request.
- Fast/Priority or Flex processing.
- A hard latency SLO or ETA before representative measurement.
- Removing Batch.

## Reviewed current implementation

### End-to-end path

1. `internal/webapp/webapp.go` and `internal/webapp/views.templ` submit the existing
   owner-scoped preparation with optional external-translation consent and poll its
   status.
2. `internal/prepareddeck/jobs.go` creates the `deck_preparations` row and the
   `prepared_deck` River job.
3. `cmd/server/main.go` currently calls `prepareddeck.AddBatchWorker`, making
   `internal/prepareddeck/cutover.go`'s `BatchPlanner` the only prepared-deck
   planner when external translation is configured.
4. `BatchPlanner` calls the existing card-export builder, freezes a durable run and
   immutable manifest through `internal/persistence/prepared_deck_runs.go`, plans
   deterministic chunks, and inserts Batch submission jobs.
5. `internal/prepareddeck/batch_submit.go` performs cache-hit reconciliation,
   uploads JSONL, creates the provider Batch, and persists remote IDs.
6. `internal/prepareddeck/batch_lifecycle.go` polls, validates custom IDs and result
   counts, decodes responses with the shared codec, persists cache rows and item
   outcomes incrementally, and schedules retries or finalization.
7. `internal/prepareddeck/recovery.go` reconstructs missing Batch, cleanup, and
   finalizer jobs. It already sees scalar `outcome` recovery rows but deliberately
   ignores them because ADR 0031 superseded that worker.
8. `internal/prepareddeck/durable.go` and
   `internal/persistence/prepared_deck_run_transitions.go` claim finalization and
   atomically complete the APKG through the existing `cardexport.Service` boundary.
9. `internal/persistence/prepared_deck_status.go`, `internal/webapp/webapp.go`, and
   `internal/webapp/views.templ` derive and render aggregate progress. Download
   remains a read-only operation in `internal/prepareddeck/jobs.go`.

### Existing reusable foundations

- `internal/enrichment/llm.go` implements the current Chat Completions client,
  request timeout, provider error type, and OpenAI-compatible base URL support.
- `internal/enrichment/llm_codec.go` is the shared Batch/standard request and
  response codec and includes the model plus prompt version in provider identity.
- `internal/enrichment/enrichment.go` defines translation requests, responses,
  immutable cache identity, retry classification, and observed cache/provider
  metrics.
- Migration `000033_durable_prepared_deck_runs` already supplies scalar outcome
  claims, leases, generations, attempts, next-attempt time, cache/provider counters,
  and recovery indexes.
- `internal/persistence/prepared_deck_run_transitions.go` already supplies claim,
  retry, finish, redispatch, and recovery operations for scalar items.
- Existing Batch tests supply strict custom-ID, missing/duplicate/unknown result,
  malformed output, partial result, cancellation, and restart fixtures that must
  continue to pass.

### Gaps

- Runs do not freeze an execution mode or target language.
- The shared cache key has no explicit target-language column; English is currently
  implicit in the prompt/provider version.
- There is no standard prepared-deck item worker, dispatcher, or recovery repair.
- The only `prepared_decks` River queue has `MaxWorkers: 1`; no dedicated bounded
  standard-provider queue exists.
- Standard enrichment retries happen in one call with a timer; durable prepared
  deck retries must instead persist `next_attempt_at` and release the worker.
- The public projection and status copy are Batch-centric.
- Finalization currently allows terminal failed translation outcomes, which does
  not satisfy requested interactive translated-deck completeness.
- Metrics are named and bounded around Batch rather than execution mode.

## Product behavior

### Initial decisions

- Existing interactive preparation selects `standard` automatically.
- `batch` is available only through explicit process/operator configuration; no new
  learner control is added.
- Target language is explicitly `en`.
- Each standard request contains one manifest item.
- The current configured model and Chat Completions compatibility rules are reused.
- The shared cache remains cross-user.
- A consented standard run fails clearly after bounded item exhaustion and does not
  publish an incomplete translated APKG.
- A preparation without translation consent preserves current local-only behavior.
- No exact ETA is displayed; users see persisted counts and phase.

### Public states

The existing public states remain:

```text
queued -> preparing -> ready
                    -> failed
                    -> cancelled
```

A standard translated run reports phase `translating` with aggregate
`completed`, `running`, `retrying`, `pending`, and `failed` counts. It reports
`finalizing` only after the required completed count equals the eligible count.
Batch-only phases and request counts remain available for explicit Batch runs.

## Architecture

### Frozen execution identity

A run records `execution_mode` (`standard` or `batch`) and `target_language`
(`en` initially). Manifest/cache identity also carries target language. Existing
cache rows backfill to `en`; provider/model/prompt version and sentence hash remain
part of the immutable key.

Mode selection happens once when the run freezes. Recovery and retries read the
persisted mode rather than current environment configuration.

### Mode-specific durable dispatch

Introduce a narrow planner/dispatcher selected by mode. It receives the prepared
manifest and commits mode-specific durable work in the same transaction as the run:

```text
standard -> item outcome jobs -> one standard request per item
batch    -> durable chunks -> submit/poll/reconcile existing Batch jobs
```

The common contract is the run, manifest, outcomes, exact cache, and finalizer—not
a forced provider-job API. Standard workers do not manufacture Batch objects, and
Batch remains outside the scalar synchronous provider interface.

### Standard item worker

One River job carries only owner, preparation, run, ordinal, and dispatch generation.
It:

1. claims the exact pending outcome with a short lease;
2. reloads and verifies the frozen run and manifest item;
3. checks the exact cache and completes immediately on a hit;
4. verifies the claim immediately before the external call;
5. makes one timeout-bounded standard request through the shared codec/client;
6. validates the opaque item ID, language pair, required fields, and strict schema;
7. commits the immutable cache result and completed outcome atomically;
8. on a retryable failure, persists bounded class/code and jittered
   `next_attempt_at`, releases the claim, and schedules the next generation;
9. on exhaustion or terminal failure, marks the item failed and fails a consented
   standard run without scheduling finalization.

A stale generation can never write a cache row or outcome after cancellation,
retry, or supersession.

### Concurrency and retry configuration

Use a dedicated `prepared_deck_translation` River queue whose worker count is
`MOUSEION_PREPARED_DECK_STANDARD_MAX_CONCURRENCY` (default `4`). This is the hard
per-process provider-call bound and avoids an unbounded goroutine per card. The
current deployment uses one worker process; aggregate capacity across future
replicas must be calculated as replicas multiplied by this setting.

Continue to use `MOUSEION_LLM_TIMEOUT` for each request. Add:

```text
MOUSEION_PREPARED_DECK_TRANSLATION_MODE=standard|batch
MOUSEION_PREPARED_DECK_STANDARD_MAX_CONCURRENCY=4
MOUSEION_PREPARED_DECK_STANDARD_MAX_ATTEMPTS=3
MOUSEION_PREPARED_DECK_STANDARD_RETRY_BASE_DELAY=1s
MOUSEION_PREPARED_DECK_STANDARD_RETRY_MAX_DELAY=30s
```

Retry HTTP 408, 429, provider 5xx, connection failures, and timeouts with bounded
exponential backoff plus jitter. Configuration, identity, cancellation, and
non-retryable 4xx errors are terminal. A malformed structured response may consume
a bounded retry attempt, but never persists as success.

### Structured output

The shared request/response schema is versioned and includes an opaque expected item
ID plus source and target language fields. Validation rejects:

- invalid JSON or extra/missing schema fields;
- a missing or mismatched item ID;
- missing, duplicate, or unexpected items if grouping is introduced later;
- a source or target language mismatch;
- empty required translation fields;
- sentence-target output that violates existing target-alignment checks.

Bump the codec prompt/schema version so older cache rows are not silently reused.
Both standard and Batch fixtures must exercise the new schema.

### Completeness and APKG boundary

For a consented `standard` run:

```text
completed outcomes == eligible outcomes
```

is a prerequisite for finalization. Any exhausted or terminal item failure fails the
run/preparation. No APKG is published. Local-only runs and explicit Batch runs retain
their separately documented policies. The finalizer, deterministic card order,
provenance writes, completeness accounting, artifact bytes, and download routes do
not otherwise change.

### Observability

Record low-cardinality execution-mode-aware measurements for:

- preparation and translation duration;
- eligible/completed/failed item counts;
- request count and latency;
- retries by bounded class, 429s, 5xx, timeouts, and validation failures;
- cache hits/misses and cache latency;
- prompt/completion token usage where the provider returns it;
- final outcome and recovery redispatches.

Never label metrics with owner, run, item, source text, provider response text, or
provider object IDs. Structured logs may carry opaque run identity only where
current operational logging policy permits it.

## File-level implementation map

| Area | Current files | Planned change |
|---|---|---|
| Decision/docs | `doc/adr/0031-*`, `doc/features/openai-batch-translation.md`, `doc/features/llm-translation-performance.md`, `doc/product.md` | Accept ADR 0032, then mark conflicting Batch-only statements superseded and update current operations after code lands. |
| Schema/domain | `migrations/000033_*`, new migration; `internal/domain/prepared_deck_run.go`; `internal/enrichment/enrichment.go` | Add frozen execution mode and target language; extend exact cache identity and constraints. |
| Persistence | `internal/persistence/enrichment.go`, `prepared_deck_runs.go`, `prepared_deck_run_transitions.go`, `prepared_deck_status.go`, `prepared_deck_batch_reconciliation.go` | Read/write the expanded identity, enforce mode-specific transitions, atomically persist standard results, recover scalar jobs, and fail incomplete standard runs. |
| Codec/provider | `internal/enrichment/llm_codec.go`, `llm.go`, `openai_batch_jsonl.go` and tests/fixtures | Add opaque item/language schema fields, strict validation, and a prompt/schema version bump shared by both transports. |
| Planning/dispatch | `internal/prepareddeck/cutover.go`, `durable.go`, new mode/planner files | Select a frozen mode; retain Batch planner; create standard per-item jobs. |
| Standard worker | new `internal/prepareddeck/standard_*.go`; `recovery.go` | One claimed item and provider attempt per River job; cache hit, strict request, persisted retry, terminal failure, and restart repair. |
| Runtime config | `internal/prepareddeck/config.go`; `cmd/server/main.go`; `internal/analysis/jobs.go`; `.env.example`; `compose.yaml` | Parse settings, create dedicated queue, wire standard provider and workers, retain Batch workers when configured. |
| Status/UI | `internal/persistence/prepared_deck_status.go`; `internal/webapp/webapp.go`; `views.templ` and generated file/tests | Expose execution mode and mode-aware phases/counts; remove Batch language from standard jobs. |
| Metrics/smoke | `internal/prepareddeck/metrics.go`; `cmd/batch-validation`; new/extended validation command; `doc/reports/` | Generalize metrics and add an explicit paid standard-path smoke/report workflow plus APKG compatibility check. |

## Dependency waves

### Wave 0 — decision review

Accept ADR 0032. Implementation issues may be prepared before acceptance but should
not merge behavior that contradicts ADR 0031 until this decision is approved.

### Wave 1 — independent foundations

1. Migration/domain/persistence identity: execution mode and target language.
2. Shared strict codec and fixtures: opaque item ID, language pair, schema version.

The migration follows Mouseion's CODEOWNER/manual merge path. These two changes can
be developed in parallel if they agree on the domain field names above.

### Wave 2 — execution path

3. Mode-selected planner/config/runtime queue.
4. Standard item worker, persisted retry, scalar recovery, and strict failure.

The item worker depends on both Wave 1 issues and the selected queue/job types from
the planner issue.

### Wave 3 — user contract and operations

5. Mode-aware status/progress and strict finalization completeness.
6. Observability, operational docs, paid smoke test, latency/cost report, and APKG
   importability evidence.

## Acceptance criteria

- [ ] The existing interactive preparation path freezes `standard` and does not
  create, upload, or submit OpenAI Batch work.
- [ ] Batch remains reachable only through an explicit non-default mode.
- [ ] Standard provider calls run concurrently with a tested hard configured bound.
- [ ] Each River item attempt performs at most one external call.
- [ ] Retryable failures persist bounded exponential backoff with jitter and survive
  restart; terminal failures do not loop.
- [ ] Strict decoding rejects malformed JSON, wrong IDs/languages, missing fields,
  duplicates, unexpected output, and empty required translations.
- [ ] Each successful result is committed to the exact immutable cache and item
  outcome before the run completes.
- [ ] Restart recovery skips completed items and repairs pending or expired scalar
  jobs without duplicate cards or stale writes.
- [ ] Cache identity explicitly includes target language and versioned provider/model/
  prompt/schema semantics; existing rows backfill safely to English.
- [ ] A consented standard run cannot become ready with a failed/missing required
  translation.
- [ ] The ready APKG and download contract remain immutable, owner-scoped, and
  importable.
- [ ] Status distinguishes standard translating from Batch waiting and reports
  honest persisted progress.
- [ ] Tests cover mode selection, queue bounds, retry timing/classification,
  malformed responses, incremental persistence, restart, stale generations, cache
  identity, finalization gating, and Batch regression.
- [ ] An explicitly acknowledged real standard-endpoint smoke test records measured
  duration, request count/latency, retries, tokens/cost, translation checks, and
  APKG importability; fake tests are not presented as latency evidence.

## Verification strategy

### Unit and contract tests

- `go test ./internal/enrichment ./internal/prepareddeck ./internal/domain`
- Codec golden fixtures for valid, malformed, mismatched, duplicate, missing, and
  unexpected IDs and language pairs.
- Injectable clock/jitter tests for backoff and retry scheduling.
- River queue tests proving no more than configured simultaneous provider calls.

### Persistence/integration tests

- `go test ./internal/persistence ./internal/prepareddeck`
- Run migration up/down coverage using the repository integration-test harness.
- Concurrent first-writer cache tests with target language in the key.
- Incremental successful writes, crash/restart, expired lease, stale generation,
  cancellation, retry exhaustion, and finalizer gating.

### Repository gates

Use the commands required by `AGENTS.md` for every implementation PR:

```bash
gofmt -w <changed-go-files>
go vet ./...
go test ./...
templ generate
make generate
make test-integration
git diff --exit-code
```

Run `make test-nlp` only when NLP service files change. Generated `*_templ.go` files
must be committed with template changes.

### Paid manual smoke

Extend or add an operator-only command that is offline by default and requires an
explicit acknowledgement before paid calls. Run a representative prepared deck
through the real standard endpoint and record:

- model, prompt/schema version, item and request count;
- wall-clock duration plus average and tail request latency;
- retries, rate limits, validation failures, cache hits/misses;
- provider token usage and configured cost calculation;
- all required translated fields present;
- generated archive/database structure and Anki importability;
- comparison with the existing Batch validation report.

Do not hard-code an SLO from one run.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Standard costs more than Batch | Shared exact cache, one item/request initially, usage telemetry, retain Batch offline |
| 429s or unstable latency | Dedicated bounded queue, configurable concurrency, persisted jittered backoff |
| Bad structured output | Shared strict schema, opaque item correlation, bounded retry, no success write |
| Worker restart | Existing leased outcomes and recovery projection; one attempt/job |
| Duplicate billing after ambiguous timeout | Generation fencing and no automatic Batch fallback; acknowledge that provider timeouts cannot make billing exactly-once |
| Incomplete translated deck | Standard-mode completeness gate fails before APKG publication |
| Cache collision across future target languages | Explicit target-language migration and prompt/schema version |
| Multiple worker replicas exceed provider limit | Document per-process bound; calculate aggregate replicas; add global permit design before scale-out |
| Batch regresses while no longer default | Keep shared codec and explicit Batch integration/regression tests |

## Related

- [ADR 0032: Make standard requests the interactive prepared-deck translation default](../adr/0032-standard-first-prepared-deck-translation.md)
- [OpenAI Batch API for prepared-deck translation](openai-batch-translation.md)
- [Durable prepared-deck translation](durable-prepared-deck-translation.md)
- [Prepared-deck translation performance](llm-translation-performance.md)
- [Prepared-deck translation operations](../../internal/prepareddeck/README.md)
