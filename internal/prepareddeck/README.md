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
