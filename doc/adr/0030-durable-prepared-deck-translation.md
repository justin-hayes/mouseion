# ADR 0030: Durable prepared-deck translation runs

Status: **Accepted** · Date: 2026-08-28 · Author: Justin + Codex · Amended by [ADR 0071](0071-decouple-deck-data-from-presentation.md)

> **Amendment:** [ADR 0031](0031-openai-batch-prepared-deck-translation.md)
> supersedes this ADR's scalar per-item provider
> jobs and leased provider permits. The immutable manifest, exact item outcomes,
> owner scoping, cancellation, and atomic finalizer remain unchanged.

## Context

ADR 0022 made deck preparation asynchronous and made the ready APKG a durable,
immutable artifact. The current prepared-deck worker still performs selection,
translation, and rendering inside one River attempt. Its manifest and
candidate outcomes exist only in memory. A process exit therefore loses all
translation progress, and a `preparing` row with no live job is failed rather
than resumed.

The exact-cache-identity work now freezes selection and rendering decisions in
memory and binds every result to the same language, lemma, UPOS, sentence hash,
provider, and provider-version key used by enrichment. That contract prevents
a wrong cache row from reaching an artifact, but it does not make the manifest
or per-candidate progress durable.

## Decision

Prepared-deck translation will be a durable, preparation-owned run between
manifest creation and artifact finalization.

1. A preparation retry creates a new run. The run owns one immutable,
   schema-versioned manifest that records selection order, accepted and omitted
   candidates, approved sentence and target decisions, render inputs, consent,
   and the exact existing enrichment-cache identity. Once committed, a run is
   never re-selected or rebound to different provider configuration.
2. Translation is decomposed into idempotent River work addressed by run and
   manifest-item identity. Candidate outcomes and attempt state are persisted
   separately from the immutable manifest. River and application leases permit
   at-least-once execution; claim tokens fence stale workers from changing a
   newer outcome.
3. Provider and cache failures are handled per candidate with bounded retry and
   backoff. Exhausting one optional translation records a terminal failed item
   but does not fail the run. The translation run is `completed` when all items
   are terminal and finalization is safe; run-level `failed` is reserved for an
   orchestration, persistence, or identity invariant that prevents a safe
   artifact. A later finalization failure leaves translation completed but
   fails the overall run and public preparation.
4. External work is dispatched through the execution layer selected by the
   accepted Batch amendment. For prepared decks, Batch chunks carry the
   provider boundary and their submission/reconciliation claims are durable.
5. One idempotent finalizer reads manifest items in frozen order, reads only the
   manifest's exact cache keys, renders once per finalizer attempt, and uses the
   existing atomic completion boundary to commit APKG bytes, completeness,
   cards, generated-vocabulary provenance, and the overall run state. A ready
   preparation is a successful idempotency result for a retried finalizer.
6. The public preparation states remain `queued`, `preparing`, `ready`,
   `failed`, and `cancelled`. Internal translation states are
   `pending`, `running`, `completed`, `failed`, and `cancelled`. Download remains
   an owner-scoped read of a ready artifact and cannot enqueue work, call a
   provider, render, or mutate study state.
7. The external privacy boundary and cache contract from ADRs 0007 and 0021 do
   not change. River arguments contain opaque run/item identifiers rather than
   sentences. Prompts, raw provider responses, and raw provider error bodies are
   not persisted as orchestration data.

The detailed state transitions, schema, retry policy, recovery cases,
and implementation sequence are specified in the
[durable prepared-deck translation design](../features/durable-prepared-deck-translation.md).

## Consequences

- A worker crash or process restart resumes a committed manifest instead of
  repeating selection or discarding completed candidate work.
- Provider calls remain at-least-once. Exact immutable cache insertion and
  fenced outcome writes make repeated execution safe, but a timeout or expired
  lease can still cause a duplicate billable request.
- Candidate rows contain owner-scoped approved source sentences and therefore
  require the same access controls and deletion treatment as other private
  source-derived data.
- The design adds persistence, River job kinds, reconciliation, and operational
  controls that allow the superseded in-memory translation loop to be removed.
- Database migrations and retention defaults require owner review. This ADR
  chooses the durable shape, not a retention duration or production concurrency
  value.

## Alternatives considered

- **Retry the whole prepared-deck job.** Rejected because it repeats selection,
  cache work, and rendering and cannot report exact candidate progress.
- **Store progress only in River metadata.** Rejected because metadata is tied
  to one job row and is not an immutable preparation manifest or a sufficient
  owner-scoped finalization contract.
- **One long-running translation job with an in-memory pool.** Rejected as the
  final architecture because a crash still loses in-flight scheduling state;
  the accepted Batch path provides durable provider scheduling instead.
- **Render partial artifacts as candidates finish.** Rejected because it breaks
  deterministic order, completeness accounting, and the atomic download
  contract.
- **Redesign the enrichment cache.** Rejected for this decision. The durable
  manifest records and consumes the exact cache key already enforced by the
  prepared-deck path.

## Related

- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0010: River job queue](0010-river-job-queue.md)
- [ADR 0012: External translation through River](0012-enrichment-execution-via-river.md)
- [ADR 0021: Contextual translation cache](0021-contextual-translation-cache.md)
- [ADR 0022: Asynchronous prepared decks](0022-prepared-decks.md)
- [Prepared-deck LLM translation performance](../archive/features/llm-translation-performance.md)
- [ADR 0031: OpenAI Batch prepared-deck translation](0031-openai-batch-prepared-deck-translation.md)
