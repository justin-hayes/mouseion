# Prepared-deck translation operations

Prepared-deck translation uses durable standard item work by default. The coordinator
freezes selection, order, sentence and target decisions, quality omissions,
and exact cache identities before any provider work. Batch submission,
polling, reconciliation, retry, cancellation, cleanup, and finalization are
short River jobs backed by persisted state.

Configure the accepted operational settings with:

- `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS` — maximum requests per input
  file, default `5000`, capped at `50000`.
- `MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL` — status polling interval,
  default `30s`, capped at `24h`.

Only the official OpenAI API and `/v1/chat/completions` endpoint are eligible
when external translation is enabled. An ineligible endpoint fails startup;
there is no runtime transport selector or Batch feature flag. Every new
preparation requires a configured provider and contextual translation. If the
provider is unavailable or translation fails, the preparation remains failed
and retryable; it never publishes a local-only deck. Previously completed
preparations remain available for download.

Progress is derived from persisted run, outcome, and Batch-chunk state. The
learner-facing status exposes aggregate counts and bounded failure classes, not
provider object IDs, prompts, responses, source sentences, credentials, or raw
provider errors. Provider files expire after seven days and are deleted
best-effort after reconciliation or cancellation.
