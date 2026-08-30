# Prepared-deck translation performance

Status: Superseded by accepted standard-first cutover · Date: 2026-08-28

This investigation recorded the latency and scheduling concerns that led to
durable prepared-deck translation and OpenAI Batch. Its former per-deck
translation scheduling is no longer a prepared-deck runtime path.

Prepared-deck preparation now freezes selection, sentence and target decisions,
quality omissions, order, and exact cache identities in a durable run. Cache
misses are executed as one durable standard request per eligible item by
default; explicit Batch work uses deterministic chunks. River workers persist,
retry, and finalize the run.

The shared translation codec remains responsible for the request body,
provider input privacy boundary, response decoding, and quality validation.
The existing synchronous client remains available to non-prepared-deck
enrichment and is the provider boundary used by the default standard worker
and operator-only validation harness.

The accepted prepared-deck operational settings are:

- `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS`, default `5000`, capped at
  `50000`.
- `MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL`, default `30s`, capped at `24h`.

Batch uses the official OpenAI API and Chat Completions endpoint only when
external translation is enabled. A custom OpenAI-compatible endpoint fails
startup with an actionable configuration error; there is no runtime transport
selector or Batch feature flag. Disabled LLM translation and preparations
without learner consent finalize without provider work.

The durable Batch design preserves the contracts established by the original
investigation: exact cache identity, deterministic artifact order and
completeness, owner isolation, cancellation fencing, and pure downloads.
Provider files are temporary and are deleted best-effort after reconciliation
or cancellation. See [OpenAI Batch API for prepared-deck translation](openai-batch-translation.md)
and [ADR 0031](../adr/0031-openai-batch-prepared-deck-translation.md) for the
current implementation and operational decisions.
