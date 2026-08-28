# Prepared-deck translation observability

Every prepared-deck worker execution emits one `prepared_deck_observation` JSON
log record. The record contains phase durations, aggregate translation/cache
outcomes, bounded error classes, concurrency, peak provider calls, and final
artifact completeness. Its schema cannot represent candidate content, user or
source identity, provider request/response bodies, credentials, or raw errors.
Durations in the production record are nanoseconds.

Run the complete deterministic benchmark matrix with one sample per case:

```bash
TMPDIR=/root/tmp-go go test ./internal/prepareddeck \
  -run '^$' \
  -bench '^BenchmarkPreparedDeckTranslation$' \
  -benchtime=1x \
  -count=1
```

The 432 cases cover 10/50/200 candidates, 0/50/100% cache hits,
100ms/500ms/1s fake-provider latency, configured concurrency 1/2/4/8, and
success/429/5xx/timeout outcomes. Each line reports request, attempt, retry,
cache, failure, completeness, configured/effective concurrency, peak in-flight,
and deterministic-artifact measurements.

The fake provider uses a virtual latency model instead of sleeping. The normal
Go `ns/op` and allocation columns measure harness overhead;
`modeled_wall_ms/op`, `provider_ms/op`, and `cache_ms/op` are the deterministic
latency baseline. This makes serial/concurrent comparisons repeatable and keeps
the full matrix practical. For a concise serial-versus-concurrent comparison:

```bash
TMPDIR=/root/tmp-go go test ./internal/prepareddeck \
  -run '^$' \
  -bench 'BenchmarkPreparedDeckTranslation/candidates_50/hits_50pct/latency_500ms/concurrency_(1|4)/outcome_success$' \
  -benchtime=1x \
  -count=1
```

Prepared-deck translation uses a bounded per-deck worker pool. Configure its
maximum in-flight enrichment operations with
`MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY`; the default is `1` for serial
compatibility. The `prepared_decks` River queue still runs one whole-deck worker,
so this setting does not increase simultaneous deck preparations.

Before translation, the worker freezes selection, order, sentence/target
decisions, quality omissions, and generated-card provenance in an immutable
in-memory manifest. It binds every accepted candidate to the configured
provider/version and sentence hash, applies only enrichment returned under
those exact identities, and renders TSV/APKG once. Provider failures leave the
corresponding optional fields empty without repeating selection or database
reads.

## Durable Batch operations

Durable prepared-deck runs expose `freezing`, `submitting`, `waiting`,
`reconciling`, `retrying`, and `finalizing` as phase details while retaining
the public `queued`, `preparing`, `ready`, `failed`, and `cancelled` states.
Progress is derived from persisted outcome and Batch-chunk rows, not River job
metadata. The learner-facing status contains aggregate request and retry
counts, translation completeness, and bounded failure classes; it never
contains provider object IDs or raw provider errors.

External translation is intentionally offline. OpenAI Batch processing can
take up to its 24-hour completion window, so
`MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL` (default `30s`) controls short
status polls and `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS` (default `5000`)
bounds each input file. Provider input/output/error files request seven-day
expiration and are deleted after reconciliation or cancellation. Cleanup
failures are persisted, observable, and retried at most three times; a cleanup
failure never rolls back already reconciled results.

## Batch validation

`cmd/batch-validation` is an operator-only evaluation command. Without
`-real-provider` it replays the checked-in frozen manifest and response fixtures
through both synchronous and Batch semantics without network access:

```bash
go run ./cmd/batch-validation -report /tmp/batch-validation.md
```

Paid calls require the explicit `-real-provider` flag and the exact
`-acknowledge-paid-provider-calls "I understand this makes paid OpenAI calls"`
acknowledgement. The command always runs both transports for the same frozen
requests; it has no `sync|batch` selector and is not used by startup, CI, or
the deployed worker. Real cost evidence additionally requires the current
model's `-input-cost-per-million` and `-output-cost-per-million` rates. See the
[report template](../../doc/reports/openai-batch-validation-report.md)
for evidence separation, cutover gates, and operator recovery records.
