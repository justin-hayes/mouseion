# Prepared-deck LLM translation performance

Status: Investigation complete · Date: 2026-08-27

This document records the verified prepared-deck translation path and the
follow-up design for improving it. It is an implementation brief for focused
GitHub issues, not a production performance report.

Issue #335 implemented the first recommendation below as a bounded per-deck
worker pool. `MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY` sets the positive
in-flight limit and defaults to `1`; the whole-deck River worker count remains
one. The investigation sections retain the serial pre-change baseline that
motivated the implementation.

## Evidence boundary

**Verified** below means established by the current source, configuration, or
tests. **Hypothesis** means an optimization expectation that still needs a
benchmark. No production latency, throughput, cache-hit, candidate-count, or
provider-rate-limit measurements are present in this repository; none are
claimed here.

## Current request flow

The relevant path is:

1. `internal/webapp/webapp.go:createDeckPreparation` accepts the analysis ID
   and the learner's external-translation consent. It calls
   `internal/prepareddeck/jobs.go:Service.Submit`, which validates an owned,
   completed scoped analysis and transactionally creates the preparation and
   River job.
2. `cmd/server/main.go` constructs one `enrichment.Service`, registers both
   `enrichmentjob.AddWorker` and `prepareddeck.AddWorker`, and starts the
   shared River client. `internal/analysis/jobs.go:NewClient` currently gives
   the `prepared_decks` queue `MaxWorkers: 1`.
3. `internal/prepareddeck/jobs.go:Worker.Work` claims the preparation, checks
   the immutable source identity, and prepares a manifest. For a scoped run,
   this reaches `internal/cardexport/cardexport.go:Service.PrepareCoverageForAnalysis`:
   it loads the immutable corpus, selects coverage candidates, loads each
   entry without a broad cache lookup, and freezes order, sentence/target
   decisions, quality omissions, and generated-card provenance.
4. When both consent and provider configuration are present, `Worker.Work`
   iterates those candidates serially and calls
   `internal/enrichment/enrichment.go:Service.EnrichExternal`. Each miss does a
   cache lookup, one provider request per candidate (with the configured retry
   policy), and an immutable cache put. The current server construction leaves
   `MaxAttempts` at the enrichment default of one; prepared-deck River jobs
   are also inserted with `MaxAttempts: 1`.
5. After the pool completes, `Worker.Work` binds the manifest to the same exact
   language/lemma/normalized-UPOS/sentence/provider/version keys used during
   enrichment, applies those returned results positionally, and renders once.
   Item translation errors are deliberately ignored so optional translation
   cannot prevent a deck from completing.
6. `internal/webapp/webapp.go:downloadDeckPreparation` calls
   `Service.Download`, which is a read of the persisted ready artifact. It does
   not call the provider, select candidates, render, or mutate study state.

The current unit test
`internal/prepareddeck/jobs_test.go:TestWorkerEnrichesManifestRendersOnceAndCompletes`
verifies one manifest preparation, one enrichment call, and one render for one
candidate. The test is behavioral verification, not a timing measurement.

## Verified bottlenecks and risks

- **Serial provider critical path.** The external loop in `Worker.Work` waits
  for candidate *i* before starting candidate *i+1*. With cache misses, deck
  preparation wall time therefore includes one provider round trip per
  accepted candidate, plus retries and backoff.
- **Duplicate preparation work (resolved by #337).** The worker now freezes a
  manifest before enrichment and renders it once afterward; selection,
  database reads, sentence gating, TSV generation, and APKG generation are not
  repeated.
- **Per-candidate cache overhead.** `EnrichExternal` performs a cache lookup
  for every candidate; on a miss, `PostgresStore.Put` inserts and reads back
  the winning immutable row. `GetCoverageEntryForCorpus` then performs
  another cache read during the final build.
- **Queue serialization.** `prepared_decks` has one River worker. Raising that
  queue limit would increase simultaneous whole-deck work and provider load; it
  would not remove the serial loop inside one deck.
- **Retry and failure behavior is conservative but under-observed.** The LLM
  client has a configured HTTP timeout (default `30s`) and recognizes 408, 429,
  and 5xx responses as temporary. The prepared path currently does not expose
  per-item progress or outcome metrics, and its server defaults mean a
  provider failure is generally a missing optional translation rather than a
  retryable prepared-deck failure.
- **Cache identity is exact in prepared rendering.** `Service.EnrichExternal` keys by
  language, canonical lemma, normalized UPOS, provider, provider version, and
  `SentenceHash`. The prepared manifest consumes the exact result returned for
  that same key and rejects provider/version, sentence, position, or provenance
  mismatches instead of performing a final broad cache lookup.

These are structural bottlenecks verified from code. Their contribution to
real elapsed time is **unknown** until the proposed benchmark and runtime
metrics exist.

## Prioritized options

1. **Immediate: bounded per-deck concurrency.** Run a small, configurable
   number of independent `EnrichExternal` calls concurrently, wait for all of
   them, and retain the existing final deterministic render. This directly
   attacks the verified serial provider path with limited implementation and
   rollback risk.
2. **Immediate companion: instrument before tuning.** Record phase durations,
   candidate/cache/provider counts, provider latency, attempts, and failure
   classes without recording sentences, lemmas, titles, owner IDs, or prompt
   contents. This establishes the missing production baseline.
3. **Implemented in #337: remove avoidable duplicate work.** Freeze an immutable candidate
   manifest after the first pass and let the finalizer render from that
   manifest plus exact cache results. Do this only after tests prove that
   selection, quality omissions, ordering, completeness, and generated-card
   provenance are unchanged.
4. **Next: reuse the existing durable enrichment shape.** Make translation a
   durable preparation phase with per-item progress and retry, rather than
   coupling all provider work to one non-resumable prepared-deck attempt.
5. **Conditional: provider batching or model/prompt changes.** These may reduce
   request count or latency, but change partial-failure, cache, cost, and
   quality behavior. They require provider-specific quality and privacy tests.
6. **Lower priority: increase River whole-deck workers.** Consider only after
   provider quotas, database pool capacity, and a global in-flight policy are
   measured. This is throughput scaling, not a fix for the per-deck bottleneck.

## Recommended bounded-concurrency change

Add a concurrency limit around only the external-enrichment loop in
`prepareddeck.Work`.

- Preserve the candidate slice and final render order. Do not assemble output
  by ranging over a map.
- Use a bounded worker pool or semaphore; never start one unbounded goroutine
  per candidate. Keep the limit configurable, with `1` as the compatibility
  setting and a small experimental value (for example `4`) evaluated by the
  benchmark matrix below. The example is a test starting point, not a claimed
  production optimum.
- Keep cache access and provider calls inside each bounded task. PostgreSQL
  handles independent connections, and the HTTP client is intended for
  concurrent use, but test doubles and any future provider implementation must
  be concurrency-safe.
- Collect errors by candidate for metrics while preserving the current policy:
  an individual optional translation failure does not fail the deck. Context
  cancellation and final-render errors remain fatal to the preparation.
- Wait for every task before the sole render. Never render or complete while
  enrichment tasks are still writing cache rows.
- Do not change cache keys, external payload fields, consent checks, or the
  provider's plain-text response validation in this slice. Fix the exact
  provider/version lookup before declaring cache-version behavior safe.
- Keep `prepared_decks` at one whole-deck worker during the first rollout. If
  that queue is later widened, add a process/global provider limiter so a
  per-deck limit cannot multiply beyond the configured provider budget.

### Rollout safeguards

Ship the limit behind configuration with a default equivalent to current
behavior. Exercise it first against a stub provider, then a canary deployment
with a low limit. Compare preparation duration, cache-hit ratio, provider
429/5xx rates, completeness, omission counts, and artifact failures against
the serial setting. Roll back to `1` if rate limits, database contention,
translation quality, or incomplete-artifact rates regress. Keep provider
timeouts and cancellation active; concurrency must not turn a stopped or
cancelled preparation into detached work.

## Longer-term durable-phase design

Use the existing `internal/enrichmentjob` model as a starting point, but bind
it to an immutable prepared-deck manifest:

1. **Freeze:** select coverage candidates, approved source sentences/targets,
   quality outcomes, export order, consent snapshot, and exact provider/cache
   identity into a preparation-owned manifest. This prevents later mutable
   learner state or re-selection from changing a retry.
2. **Translate:** enqueue a durable batch or item set. Process cache misses with
   bounded concurrency, persist per-item status and attempts, and reuse the
   immutable cross-user cache. Cache hits complete without provider calls.
   Cancellation stops new work and records a resumable or cancelled state.
3. **Finalize:** after translation work reaches a terminal condition, read the
   exact cache keys from the manifest, render TSV/APKG once, calculate
   completeness from the actual fields, persist the immutable artifact and
   generated-card provenance atomically, and expose it through the existing
   pure download endpoint.

The public preparation can retain `queued → preparing → ready/failed/cancelled`
while translation progress is internal, or introduce a separately reviewed
substate if the UI needs to distinguish freezing, translating, and finalizing.
The existing `enrichmentjob.Worker` demonstrates ordered progress and retry
resume, but its current metadata is batch-level and its job arguments are not
an immutable preparation manifest; those gaps must be resolved before reuse.

## Constraints that must survive optimization

- **Reliability:** translation is optional; missing translation must produce an
  accurate completeness count. Provider failures, cancellation, worker death,
  and retry must not create generated-vocabulary records before the final
  artifact is committed. Owner/source/analysis identity checks remain intact.
- **Privacy:** the provider boundary remains limited to language, canonical
  lemma, UPOS, tested surface, and the approved sentence. Never send owner,
  book, title, corpus, reading history, or preparation metadata. Metrics must
  be aggregate and metadata-safe.
- **Cache:** preserve sentence-sensitive, provider/versioned, immutable cache
  identity. Different sentences for one lemma must not share a result. Resolve
  the final-render lookup mismatch noted above, preferably by using the same
  exact cache abstraction/key as enrichment.
- **Determinism:** selection and card order remain based on the existing
  stable occurrence/key ordering; sentence quality and target highlighting
  remain deterministic and provider HTML remains untrusted. Concurrent cache
  misses may race to become the immutable first writer, so a durable design
  should add per-key single-flight/claiming or explicitly measure and accept
  that race.
- **Artifact contract:** the recognition-card seven-field output,
  `EnglishSentence` fallback behavior, APKG/TSV escaping, and atomic ready
  artifact semantics remain unchanged.

## Metrics, tests, and benchmark matrix

### Metrics

Add preparation-level timings for queue wait, claim, initial build,
translation phase, final build, commit, and total duration. Add counts for
selected/accepted/omitted candidates, translation-eligible candidates,
cache hits/misses, provider calls, attempts, retries, cancellations, errors by
class, and final completeness fields. Add provider and cache latency
histograms, configured/effective concurrency, and peak in-flight calls. Use
preparation IDs only as non-content correlation identifiers; never label
metrics with sentence text, lemma, title, owner, API key, or prompt/response
content.

### Tests

- Unit-test the concurrency ceiling, all-candidate completion, stable error
  handling, cancellation, and compatibility at limit `1` with a
  synchronization-aware fake provider.
- Integration-test concurrent cache misses, immutable first-writer behavior,
  sentence separation, provider-version separation, and the exact cache row
  used by final rendering.
- Assert that serial and concurrent modes produce the same selected identities,
  card order, quality omissions, completeness, and generated provenance when
  cache responses are fixed.
- Use a local HTTP stub to exercise latency, 429/5xx, timeouts, malformed JSON,
  and privacy payload assertions. Do not call a real provider in tests.
- For the durable design, test restart/resume, partial progress, cancellation,
  retry idempotency, and atomic finalization.

### Proposed benchmark matrix

This is a synthetic test matrix; it contains no production measurements.

| Dimension | Values to run |
|---|---|
| Per-deck concurrency | 1, 2, 4, 8 |
| Accepted candidates | 8, 32, 128 |
| Cache-hit ratio | 0%, 50%, 100% |
| Stub provider latency | 20ms, 250ms, 2s, and a sampled latency distribution |
| Provider outcomes | success, 429/5xx retry, timeout, permanent 4xx, malformed response |
| Deployment shape | one prepared job; then multiple whole-deck workers only after the first result is understood |

Record wall time, provider calls, attempts, cache operations, peak in-flight
calls, errors, completeness, and byte-for-byte artifact stability where the
provider responses are fixed. Select the rollout limit from these results and
observed provider/database budgets, not from the matrix alone.

## Explicit unknowns

- Real accepted-candidate distribution per prepared deck and its p50/p95/p99.
- Production cache-hit ratio and cache contention, including duplicate misses
  for the same key across users or preparations.
- Provider latency distribution, concurrency/rate limits, timeout behavior,
  and whether the configured endpoint safely supports parallel requests.
- Database connection-pool headroom and the cost of the repeated candidate and
  cache queries under concurrent work.
- Translation-quality and target-alignment changes, if any, as concurrency,
  model, reasoning effort, or batching changes.
- Whether optional per-item failures should remain invisible to preparation
  status or become resumable item-level states in the durable phase.
- Whether exact reproducibility requires cache-key claiming/single-flight, or
  whether immutable first-writer semantics are sufficient for this product.
- The retention/cleanup policy for preparation manifests and item-level
  translation history.

## Related decisions and implementation references

- [ADR 0012: external translation through River](../adr/0012-enrichment-execution-via-river.md)
- [ADR 0021: contextual translation cache and privacy](../adr/0021-contextual-translation-cache.md)
- [ADR 0022: asynchronous deck preparation and durable APKG artifacts](../adr/0022-prepared-decks.md)
- `internal/prepareddeck/jobs.go:Worker.Work`, `buildManifest`
- `internal/enrichment/enrichment.go:Service.EnrichExternal`
- `internal/enrichment/llm.go:OpenAITranslationClient.Translate`
- `internal/enrichmentjob/jobs.go:Worker.Work`
- `internal/persistence/cardexport.go:GetCoverageEntryForCorpus`
- `internal/cardexport/cardexport.go:Service.PrepareCoverageForAnalysis`, `Service.RenderManifest`
