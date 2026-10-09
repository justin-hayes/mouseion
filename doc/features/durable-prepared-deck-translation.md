# Durable prepared-deck translation

Status: Accepted · Date: 2026-08-28 · Issue: #338

This document is the implementation contract for making prepared-deck
translation durable and resumable. It refines the longer-term recommendation in
[Prepared-deck LLM translation performance](../archive/features/llm-translation-performance.md)
and [ADR 0030](../adr/0030-durable-prepared-deck-translation.md).

The implementation uses the accepted ADR 0032 standard-first execution:
prepared-deck cache misses are executed as durable standard item work by
default and reconciled through the common run/finalizer state. Explicit Batch
execution remains supported for offline/economy work.

## Decision and approval notation

- **Decision** means part of the accepted implementation contract. Acceptance of
  ADR 0030 and this design approves that contract.
- **Approval required** identifies an operational or retention default that is
  intentionally not selected by this issue.
- **Future option** is compatible with the design but is not required for the
  initial implementation.

## Problem

Before the Batch cutover, the `prepared_deck` River worker claimed a public
preparation, built an in-memory `cardexport.Manifest`, performed cache/provider
calls in a bounded in-memory pool, rendered once, and committed the final
artifact. Prepared-deck jobs were inserted with `MaxAttempts: 1`. The accepted
implementation now freezes the manifest, submits Batch chunks, reconciles
results durably, and finalizes through the same atomic artifact boundary.

This path has correct exact-cache binding and atomic completion, but the
manifest and progress disappear when the process exits. Reconciliation can
re-enqueue a `queued` preparation, but deliberately fails an orphaned
`preparing` preparation because no durable attempt lease exists. River metadata
in the former `internal/enrichmentjob` package demonstrated ordered progress, but
its bulk args contained mutable work payloads and were not bound to a
preparation manifest.

The result is a mismatch: the APKG is durable after completion, but the slowest
phase that produces it is not durable before completion.

## Goals

- Persist one immutable render and translation manifest for each logical
  preparation run.
- Resume candidate work after a worker crash, provider timeout, or process
  restart without re-selection or repeating completed work.
- Make candidate progress, attempts, cancellation, and terminal failures exact
  and owner-scoped.
- Preserve the existing cache key and immutable first-writer behavior.
- Bound submitted work with the accepted Batch request and retry limits.
- Finalize deterministically through the existing all-or-nothing APKG, card,
  completeness, and generated-vocabulary transaction.
- Preserve the current public preparation and pure-download contracts.
- Make recovery and idempotency behavior directly testable.

## Non-goals

- Synchronous prepared-deck provider calls or a second prepared-deck transport.
- A new enrichment-cache key, mutable cache entries, cache invalidation, or a
  cache single-flight redesign beyond the exact identity already implemented.
- Changing coverage selection, sentence quality, recognition-card fields,
  target highlighting, APKG layout, or generated-vocabulary meaning.
- Making translation mandatory for a deck to become ready.
- Increasing whole-deck throughput before provider and database limits are
  measured.
- Changing the download endpoint or exposing a partial APKG.
- Selecting a production retention duration or provider scheduling policy in this
  design issue.

## Existing contracts that remain authoritative

1. `deck_preparations.id` is owner-scoped and stable for one source/analysis
   identity. Repeated submission returns that preparation.
2. A manual retry moves a failed or cancelled public preparation back to
   `queued`. It does not mutate an earlier terminal run.
3. A completed scoped analysis and its immutable content, scope, snapshot, and
   corpus identities remain the only input to prepared-deck selection.
4. The manifest freezes selection, final order, approved full sentence and
   target, quality omissions, and generated-card inputs before translation.
5. The exact external cache key is the existing tuple:

   ```text
   (language, canonical_lemma, uppercase_upos,
    provider, provider_version, sentence_hash)
   ```

   `sentence_hash` remains the lowercase SHA-256 of the complete sentence after
   CRLF-to-LF conversion and outer Unicode-space trimming. The tested target is
   a frozen provider input but is not added to the existing cache key by this
   design.
6. External calls require both the submitted consent snapshot and configured
   provider availability. They contain only language, canonical lemma, UPOS,
   tested target, and the approved complete sentence.
7. Translation and target alignment are optional. Source-sentence quality is
   deterministic and is not revisited after a provider failure.
8. `CompletePreparedDeck` is the sole final commit boundary. Download returns
   bytes only when the owner-scoped preparation is `ready`.

## Identity model and inputs

### Preparation and run identity

**Decision:** retain the public preparation ID as the stable user-facing
identity and add an immutable `run_id` for each logical attempt.

- The initial successful freeze creates run number 1. A manual retry creates
  run number 2, and so on.
- Automatic River retry, crash recovery, provider retry, and finalizer retry
  retain the same run ID.
- A run becomes visible only when its header, complete manifest, candidate
  outcomes, public `current_run_id`, and initial River jobs commit in one
  PostgreSQL transaction. A crash before that commit leaves no partial run; the
  coordinator may freeze again.
- The freeze reads selection and render inputs in one owner-scoped
  `REPEATABLE READ` transaction. Once committed, later vocabulary, source
  metadata, consent, or provider configuration changes cannot alter the run.
- A manual retry deliberately creates a new snapshot, matching today's retry
  behavior. Earlier runs remain immutable history.

### Consent and provider snapshot

The run records:

- submitted external-translation consent;
- whether external translation was configured at freeze time;
- context mode;
- provider name and provider version when translation is requested;
- the retry-policy version and manifest schema version; and
- the configured attempt and concurrency limits used for the run.

If consent is false or no provider is configured, the run contains no external
cache identities, creates no provider jobs, and completes its translation phase
immediately. Enabling a provider later cannot change that run. A manual retry
may create a new run under the new snapshot.

### Immutable manifest

The manifest header records deck name, filename, counts, schema version, and a
SHA-256 digest of canonical header and item data. Every selected candidate has
one ordinal row. Accepted rows contain all provider-independent fields needed
to reconstruct the private `cardexport.Entry` and generated-card plan:

- language, canonical lemma, normalized UPOS, occurrence/export order, and
  first encounter;
- approved complete source sentence and tested target;
- morphology, source-document title, and notes used by the renderer;
- quality outcome, score, and bounded reason codes;
- accepted or `quality_omitted` disposition; and
- for translation-eligible rows, provider, provider version, and exact
  sentence hash.

The manifest stores no translation result. Translation text and its provenance
remain in `enrichment_cache`; a completed outcome states only that the exact
row is available. Canonical digest generation must be versioned and tested so
map ordering or a Go struct refactor cannot silently change identity.

## Pipeline

```text
submit/retry
    |
    v
prepared_deck coordinator
    |-- validate owner + completed analysis
    |-- freeze run + immutable manifest in REPEATABLE READ transaction
    |-- create Batch chunks and River work in the same transaction
    v
Batch chunks -- exact cache hit --> completed outcome
    |                             \
    |                              provider Batch result --> immutable cache put
    |                                                        + completed outcomes
    |                              retry/exhaustion -> pending/failed outcomes
    v
all outcomes terminal
    |-- mark translation run completed
    |-- enqueue one finalizer transactionally
    v
finalizer -- read manifest order + exact cache rows -- render once
    |-- atomically commit APKG + completeness + cards + provenance + run state
    v
pure ready-artifact download
```

### Freeze

The coordinator replaces the current one-shot claim behavior with an
idempotent orchestration claim:

1. Lock the owner-scoped preparation.
2. Return successfully for `ready` or `cancelled`; reject a contradictory
   identity.
3. Move `queued` to `preparing`. A `preparing` row with a committed current run
   is reconciled rather than failed.
4. Build the manifest using transaction-bound persistence reads.
5. Compute exact cache keys before any candidate job is created.
6. Insert the run, manifest items, outcome rows, Batch chunk configuration,
   and River jobs with `InsertTx`; set `current_run_id` in the same transaction.

An existing run with the expected digest is an idempotent success. A different
digest for the same run is an invariant failure; it must never overwrite the
manifest.

### Translate

The Batch implementation creates one durable chunk per consecutive set of
accepted, translation-eligible manifest items. River args contain only
owner, preparation, run, chunk, and generation identities; they do not contain
sentence or card content.

Each job:

1. locks and validates the owner/run/item/outcome identity;
2. skips a completed, failed, cancelled, or superseded generation;
3. claims the item with a random fencing token and a bounded lease;
4. checks cancellation, then reads the exact cache key;
5. completes immediately on an exact cache hit;
6. otherwise submits or reconciles a leased provider Batch, rechecking
   cancellation and its claim;
7. writes through the existing immutable cache; and
8. updates the outcome only when the run is still active and the fencing token
   still matches.

A cache put followed by a crash is safe: the next attempt finds the exact row
and completes without another provider call. Because an external request cannot
be transactional, provider invocation is at-least-once, not exactly-once.

### Complete the translation phase

The reconciliation worker that makes an outcome terminal locks the run and tests whether any
outcome remains `pending` or `running`. The winner:

- changes the run's translation state to `completed`;
- changes the overall run state from `translating` to `finalizing`;
- records aggregate completed/failed counts; and
- inserts the unique finalizer job in the same transaction.

Candidate `failed` is terminal and means optional English fields will be empty.
Run-level `failed` means the manifest or orchestration cannot be trusted or
finalized. This distinction preserves today's optional-translation behavior.

### Finalize

The finalizer locks the current owner/preparation/run relation, obtains a
fenced finalization claim, and then:

1. verifies the manifest digest and that translation is `completed`;
2. rebuilds the in-memory manifest strictly by ordinal;
3. for each completed outcome, reads only its exact `enrichment_cache` key and
   verifies result provenance; failed outcomes contribute empty optional
   fields;
4. calls `RenderManifest` once for that attempt;
5. calculates completeness from the rendered fields; and
6. extends `CompletePreparedDeck` so the APKG, cards, generated-vocabulary
   transitions, completeness, preparation `ready`, and run finalization state
   commit in the same transaction.

If the database commit succeeds but River does not record job completion, the
retry sees the matching ready preparation and returns success. If rendering
succeeds but the database commit fails, no artifact is downloadable and the
same run may render and commit again. A different artifact for an already ready
run is an immutable-identity error, as it is today.

## State machines

### Public preparation

The existing state machine is unchanged:

```text
queued -> preparing -> ready
                    -> failed
queued/preparing    -> cancelled
failed/cancelled    -> queued (manual retry creates a new run)
```

`preparing` covers freeze, translation, and finalization. Phase-specific
progress is returned as additional status data without adding a public state.

### Overall preparation run

```text
translating -> finalizing -> completed
      |             |
      +-------------+-> failed
      +-------------+-> cancelled
```

The overall state owns finalization and terminal run semantics. Translation can
remain `completed` when rendering or the atomic commit later fails. Cancellation
during finalization moves the overall run to `cancelled` without rewriting the
already completed translation history.

### Translation run

| State | Meaning | Allowed next state |
|---|---|---|
| `pending` | Manifest committed; candidate work exists but none is claimed. Zero-work runs may pass through this state transactionally. | `running`, `completed`, `failed`, `cancelled` |
| `running` | At least one candidate has been claimed or retried; progress is durable. | `completed`, `failed`, `cancelled` |
| `completed` | Every candidate outcome is terminal; finalization is safe even when some optional items failed. | Terminal |
| `failed` | A translation-phase persistence, identity, or orchestration invariant prevents safe finalization. | Terminal; overall run also fails and manual preparation retry creates another run |
| `cancelled` | Logical cancellation won before translation completed; no outcome from this run may be published. | Terminal; overall run also cancels and manual preparation retry creates another run |

State changes use conditional updates under a run-row lock and append a
content-free `processing_history` event. Repeating the current transition is an
idempotent success; all other invalid transitions fail closed.

### Candidate outcome

| State | Meaning | Allowed next state |
|---|---|---|
| `pending` | Not claimed, or scheduled for another provider attempt at `next_attempt_at`. | `running`, `cancelled` |
| `running` | Claimed by one dispatch generation and fencing token until `lease_expires_at`. | `completed`, `pending`, `failed`, `cancelled` |
| `completed` | The exact immutable cache row exists and may be consumed. | Terminal |
| `failed` | A permanent error or exhausted retry budget; finalization leaves optional fields empty. | Terminal |
| `cancelled` | Work was not started or its late result was rejected after run cancellation. | Terminal |

The progress numerator is terminal `completed + failed`; cancelled runs report
cancelled items separately. It is never inferred from River job metadata.

For the standard contextual-Gloss contract, a validated response may complete
with a bounded `omission_reason` instead of a cache result when it cannot give a
defensible contextual meaning. Finalization omits that manifest item, reports
its frozen target and reason, and writes no Generated vocabulary provenance for
it. Provider outages, malformed responses, and invalid evidence references do
not carry an omission reason: they remain failed translation outcomes and the
standard preparation fails closed. If every accepted target is explicitly
unresolved, finalization fails rather than publishing an empty artifact; a
selection that was already empty is not an all-unresolved run.

## Persistence schema

Names may be shortened mechanically during implementation, but the following
columns and invariants are required.

### `deck_preparation_runs`

- `id uuid primary key`
- `owner_id uuid not null`
- `preparation_id uuid not null`
- `run_number integer not null check (run_number > 0)`
- overall `state text not null check (state in ('translating','finalizing','completed','failed','cancelled'))`
- `translation_state text not null check (translation_state in ('pending','running','completed','failed','cancelled'))`
- `external_translation_consent boolean not null`
- `external_translation_configured boolean not null`
- nullable `context_mode`, `provider`, and `provider_version`, constrained to be
  present together only when consent and configuration make translation
  requested
- `manifest_schema_version integer not null`
- `retry_policy_version integer not null`
- positive configured provider-attempt and global-concurrency limits
- nonnegative aggregate `candidate_count`, `completed_count`, and
  `failed_count`, with completed plus failed never exceeding candidates
- finalization claim token/lease, bounded `error_class`, and lifecycle timestamps
- composite foreign key `(owner_id, preparation_id)` to
  `deck_preparations(owner_id, id)` with cascade deletion
- `unique(owner_id, preparation_id, run_number)` and
  `unique(owner_id, preparation_id, id)`

Add `deck_preparations.current_run_id uuid null` and a composite foreign key
that proves the current run belongs to the same owner and preparation.

Indexes:

- `(owner_id, preparation_id, run_number desc)` for status/history;
- `(state, updated_at)` restricted to nonterminal overall runs for stuck
  detection; and
- `(finalization_lease_expires_at)` restricted to claimed finalizers.

### `deck_preparation_manifests`

One row per run:

- owner, preparation, and run composite identity;
- `schema_version`, `manifest_digest`, deck name, filename, and selected,
  accepted, and omitted counts; and
- creation timestamp.

The digest is 64 lowercase hexadecimal characters. A trigger rejects updates.
Application writes use insert-only interfaces; deletion is restricted to the
later approved retention path and owner/preparation cascade cleanup.

### `deck_preparation_manifest_items`

- owner/preparation/run identity and `ordinal integer check (ordinal >= 0)`;
- `disposition` constrained to `accepted` or `quality_omitted`;
- language, canonical lemma, uppercase UPOS, source sentence, tested target,
  first encounter, quality score, bounded quality reason codes;
- schema-versioned `render_payload jsonb` containing the remaining immutable
  provider-independent `cardexport.Entry` fields, including the sentence
  dependency parse (`SentenceTokens`) used to resolve multi-span target bolding
  at finalization;
- nullable provider, provider version, and sentence hash, all present or all
  absent; and
- `candidate_digest` over canonical immutable item data.

Primary key `(run_id, ordinal)`. Add unique
`(run_id, candidate_digest)` and owner-preserving composite foreign keys. The
exact cache columns are indexed only if finalizer query measurements justify
it; finalization primarily scans the run's ordinal primary key.

### `deck_preparation_translation_outcomes`

- `(run_id, ordinal)` primary and foreign key to the immutable item;
- state, application `dispatch_count`, `provider_attempt_count`,
  `max_provider_attempts`, `next_attempt_at`;
- `dispatch_generation`, `river_job_id`, random `claim_token`, claim and lease
  timestamps;
- terminal timestamp, bounded `error_class`, and optional sanitized error code;
- cache-hit/provider-call counters and bounded latency totals needed for
  aggregate observability; and
- no prompt, response, translation text, raw error, owner name, or source title.

Indexes:

- `(state, next_attempt_at)` restricted to `pending`;
- `(state, lease_expires_at)` restricted to `running`;
- unique `(river_job_id)` when non-null; and
- `(run_id, state)` for progress and terminal checks.

### `deck_preparation_batch_chunks` and `deck_preparation_batch_chunk_items`

Durable Batch submission and reconciliation state:

- owner, preparation, run, chunk, and generation identity;
- chunk ordinal range, input digest, request count, serialized input bytes,
  estimated prompt tokens, and bounded split reason;
- provider model, endpoint, input/batch/output/error file IDs, bounded status,
  aggregate counts, usage, timestamps, and cleanup state; and
- immutable chunk-to-manifest-item mappings by ordinal, with no owner or source
  metadata in River arguments.

Submission, polling, reconciliation, and provider-file cleanup use leased
claims. Failed or expired items are assigned to a new Batch generation without
resubmitting successful items.

### Migration order

1. Create run, manifest, item, outcome, and permit tables with owner-preserving
   foreign keys and checks.
2. Add `deck_preparations.current_run_id` and its composite foreign key after
   the run table exists.
3. Add indexes and immutability triggers.
4. Deploy the durable Batch application code; new nullable columns require no
   historical backfill.
5. Enable Batch preparation after the accepted validation gate and remove the
   superseded scalar path.

The down migration must remove the current-run foreign key/column before the
new tables. No migration may rewrite existing ready artifacts or cache rows.
Migrations require CODEOWNERS review.

## Job decomposition, claims, and idempotency

| Job kind | Stable args | Idempotency result |
|---|---|---|
| existing `prepared_deck` coordinator | preparation and immutable analysis identity | committed current run or already terminal preparation |
| `prepared_deck_translation_item` | owner, run, ordinal, dispatch generation | terminal outcome or superseded generation |
| `prepared_deck_finalize` | owner, preparation, run | matching ready artifact/run finalization |
| periodic `prepared_deck_reconcile` | no content-bearing args | repairs only expired/missing work selected from persisted state |

River jobs use `MaxAttempts: 5`. Infrastructure failures use the current River
exponential schedule (approximately 1, 16, 81, and 256 seconds before jitter).
Tests pin the intended schedule so a River upgrade cannot silently change the
operational contract. Candidate provider attempts are application-controlled
so their count and next time are durable even when River snoozes do not count
as attempts.

Accepted default policy:

- maximum five provider attempts per candidate;
- retry timeout, HTTP 408, 429, 5xx, transient transport, and cache availability
  errors;
- do not retry other 4xx, schema/validation failures, or identity violations;
- exponential delays of `1s * 2^(attempt-1)`, capped at five minutes, with full
  jitter; and
- when a typed provider exposes a valid `Retry-After`, wait at least that long,
  capped at fifteen minutes.

**Approval required:** owner/operator approval of the production attempt budget,
delay caps, global permit count, and whether provider-specific budgets need
separate limiter keys. These are cost and quota choices. Tests use injected
clocks and deterministic jitter.

The item lease is longer than one provider timeout plus database commit margin.
The permit lease is at least twice the provider timeout and is renewed every
third of its duration. Loss of either lease cancels the call context. Every
post-call mutation checks run state, dispatch generation, and claim token.
These guards protect application state; they cannot guarantee that a remote
provider did not process a timed-out request.

The reconciler periodically finds:

- pending outcomes whose job is absent or terminal;
- running outcomes with an expired application lease;
- nonterminal runs with no actionable outcomes;
- completed translation runs without a live/finalized finalizer; and
- public `preparing` rows whose current run is missing or terminal.

It increments `dispatch_generation` under lock and transactionally enqueues
replacement work. Old generations become no-ops. Reconciliation is proactive;
status polling may invoke the same idempotent routine but is not required for
recovery. At most three replacement generations are created for one stuck item
or finalizer; exhausting that repair budget makes an item terminal failed or,
for coordinator/finalizer work, fails the overall run instead of looping
forever.

## Cancellation

Cancellation is a logical database transition first and a River cancellation
request second:

1. Lock the owner-scoped preparation and current run.
2. Move the public preparation and nonterminal run to `cancelled`; move pending
   outcomes to `cancelled` in the same transaction.
3. Commit, then best-effort cancel live candidate/finalizer River jobs.
4. Running workers observe cancellation through context and database checks.
   Late writes cannot pass the run-state and fencing-token predicates.

No finalizer is enqueued for a cancelled run. A provider call already received
cannot be recalled; it may still win an immutable cache insert, but its outcome
is not applied to the cancelled run. A later consented run may use the same
exact shared cache row. A later run without consent neither reads nor applies
external cache results.

## Recovery semantics

| Failure point | Durable state | Recovery and observable result |
|---|---|---|
| Process exits before freeze transaction commits | no run/manifest is visible | coordinator retry freezes and commits one run |
| Process exits after freeze commit | manifest and pending outcomes exist with jobs atomically inserted | candidate jobs continue; reconciler repairs missing/terminal work |
| Worker exits before provider call | running lease eventually expires | a new dispatch generation reclaims the item |
| Provider times out or returns retryable failure | attempt and next time persist; no completed outcome | same run resumes after backoff |
| Provider succeeds and cache put commits, then worker exits | exact cache row exists; outcome may still be running | retry converts the cache hit to completed without another call |
| Provider response is permanent-invalid or attempts exhaust | candidate becomes failed | run may still complete; final artifact reports the missing optional fields |
| Process restarts with partial completion | completed/failed rows remain terminal | only pending or expired-running candidates execute |
| Cancellation races a provider result | run cancellation and fencing predicates win publication | no artifact or provenance from the cancelled run |
| Finalizer exits before DB commit | no ready artifact is visible | finalizer retries and deterministically renders again |
| Finalizer DB commit succeeds but River acknowledgement is lost | preparation and run are complete atomically | retry returns success without republishing |
| Exact completed cache row is missing or mismatched | manifest invariant is broken | run and preparation fail closed; no partial download |

## Artifact publication and determinism

- The manifest is the only final render input; finalization does not query
  mutable selection, known-vocabulary, reserved-vocabulary, sentence-choice, or
  provider configuration state.
- Candidate order is the stored ordinal. Concurrent completion order is never
  used.
- A failed optional outcome is represented by an empty exact result, not a
  broad or lemma-only cache lookup.
- All item workers must be terminal before rendering begins. No cache/provider
  work started for the run remains in flight under a valid claim.
- APKG bytes exist outside PostgreSQL only in finalizer memory. The existing
  `DownloadDeckPreparation` state predicate prevents partial bytes from being
  returned.
- Ready state, bytes, completeness, cards, generated vocabulary, processing
  history, and run finalization commit together. A rollback publishes none of
  them.

## Compatibility and policy boundaries

### Owner scoping

Every service method begins with owner plus preparation/run identity. Composite
foreign keys prevent an item, run, or current-run pointer from crossing owners.
A foreign owner's UUID returns not found, consistent with existing prepared
decks and enrichment-job status.

### Consent and privacy

Consent is snapshotted before any job is created. Candidate job args and
metrics never carry content. The provider request type remains the compile-time
privacy boundary and must not gain owner, preparation, book, title, corpus,
scope, reading-history, or campaign fields.

### Language capabilities

The run inherits the language of the completed analysis that was already
validated against NLP capabilities. A restart does not re-query live
capabilities and therefore cannot invalidate an immutable completed analysis.
The provider/cache identity retains the exact language code from the manifest;
no German-only constraints are introduced.

### Generated-vocabulary provenance

No translation worker writes cards, decks, vocabulary state, or generated
history. The finalizer reconstructs the same `GeneratedRecord` values from the
manifest and persists them only through `CompletePreparedDeck`. Failed or
cancelled runs never exclude vocabulary from later selection.

### Pure download

`GET` remains a state-checked artifact read. Reconciliation belongs to status
and background orchestration, never download. Repeated downloads return the
same stored bytes and do not extend retention implicitly.

## Observability and operational controls

Emit structured, content-free events for run created, translation started,
candidate attempt aggregate, retry scheduled, translation terminal, cancellation,
reconciliation repair, finalization started, and finalization terminal. Event
fields may include run/preparation IDs for correlation, bounded state/error
classes, counts, attempt number, durations, queue wait, and configured/effective
limits. They must exclude candidate identity, sentences, titles, prompts,
responses, raw errors, credentials, and owner identifiers.

Required metrics:

- runs and candidates by bounded state/outcome;
- queue wait, freeze, cache, provider, translation, finalization, commit, and
  end-to-end durations;
- cache hits/misses, provider calls, retries, exhausted items, cancellations,
  reconciled jobs, expired leases, and fencing rejections;
- configured permits, claimed permits, peak provider calls, and permit wait;
- stuck nonterminal run and expired-running item gauges; and
- final completeness counts and artifact commit failures.

Metrics labels must remain low cardinality: job kind, phase, state, error class,
provider name, and provider-version rollout only when cardinality is bounded.
IDs belong in structured events, not metric labels.

Operational controls:

- Batch request limits use the approved 5,000-request ceiling and 30-second
  polling interval;
- worker timeout, application lease, provider attempt budget, and backoff are
  explicit configuration with validation;
- a stuck-run alert fires when no progress or heartbeat occurs beyond the
  derived lease/retry horizon, not merely because a large run is old; and
- a code revert preserves already committed Batch runs, which can drain or be
  cancelled.

## Security, privacy, and retention

- Manifest source sentences are private owner-scoped source-derived data. They
  receive database access controls, backup protection, cascade deletion, and
  the same incident treatment as `example_sentences`.
- Do not persist rendered prompts, request bodies, raw provider responses, API
  keys, authorization headers, provider error bodies, or owner/source metadata
  in outcomes, River args, events, or metrics.
- Translation text remains only in the existing owner-independent immutable
  cache and the final owner-scoped artifact/card data. Orchestration tables do
  not duplicate it.
- Store bounded error classes and sanitized internal codes. User-facing errors
  are generic; diagnostic logs must not interpolate raw provider bodies.
- Provider request IDs, token accounting, and cost metadata are not persisted
  initially. Adding them requires a field-by-field privacy and cardinality
  review.
- Cancellation cannot erase a request already received by an external
  provider. UI/help text should continue to describe consent as permission to
  send the approved fields, not a revocable remote deletion guarantee.

**Approval required:** retention duration. To preserve existing behavior, the
initial implementation performs no automatic deletion. A follow-up proposal
should separately choose retention for failed/cancelled superseded manifests,
ready artifacts, and the shared enrichment cache. Cleanup must never delete a
manifest or exact cache row needed by a live run or retained ready artifact,
and must expose audit counts before destructive deletion is enabled. This is
separate from the accepted seven-day expiration requested for temporary
provider files.

## Batch execution

The durable unit remains one manifest item and one exact cache identity. The
accepted Batch worker:

- map responses by candidate digest rather than response position;
- validate and commit each successful item independently;
- leave missing, duplicated, malformed, or retryable items pending for a new
  Batch generation;
- never roll back successful cache rows because another batch item failed; and
- never invoke a synchronous prepared-deck fallback.

No batch response may be published directly to a manifest or artifact.

## Verification contract

Implementation PRs must include deterministic unit tests and PostgreSQL/River
integration tests for:

- immutable manifest digest and owner/run identity;
- concurrent duplicate submission and dispatch generations;
- exact sentence/provider/version cache separation and no broad fallback;
- cache hit, successful miss, retryable timeout/429/5xx, permanent failure,
  exhausted attempts, and partial completion;
- crash before provider, crash after cache put, process restart, expired lease,
  and proactive reconciliation;
- cancellation before claim, during provider work, after candidate completion,
  and racing finalization;
- bounded concurrency across simultaneous decks and worker processes;
- deterministic candidate order and byte-stable final artifact for fixed
  inputs;
- finalization rollback at every write boundary, retry after commit, and no
  generated provenance before ready;
- owner isolation, consent false/provider disabled, German and Italian
  manifests, privacy-safe job args/events/errors, and pure repeated download;
  and
- Batch-only restart with a mix of completed, retryable, cancelled, and
  finalizing durable preparations.

Tests use local stub providers and injected clocks; they never call a real
external provider. Documentation verification must check all relative links
and paths plus `git diff --check`.

## Phased follow-up issue and PR breakdown

Each slice is independently reviewable. Later slices may be developed in
parallel only after their declared schema/interface dependency lands.

### A. Persist durable run and manifest contracts

**Depends on:** approval of ADR 0030 and this design.

Add the migration, domain types, transaction-bound freeze persistence
interfaces, immutable manifest codec/digest, outcome transition functions, and
owner-scoped queries. Do not register new production workers.

Acceptance criteria:

- migrations enforce the accepted composite ownership, state checks,
  uniqueness, indexes, and manifest immutability;
- manifest round trips reconstruct the current `cardexport.Manifest` inputs
  without external fields;
- digest/version fixtures are deterministic;
- transition and cross-owner integration tests pass; and
- existing preparations and cache rows require no backfill and behave
  unchanged.

This migration PR requires CODEOWNERS review.

### B. Freeze manifests and dispatch Batch work transactionally

**Depends on:** A.

Make the prepared-deck coordinator resumable, freeze through one repeatable-read
transaction, snapshot consent/provider identity, create outcomes, and insert
Batch chunk jobs with the run.

Acceptance criteria:

- no committed run can lack its complete manifest and initial jobs;
- coordinator retry reuses a committed run and never reselects it;
- crash-before-commit and duplicate coordinator tests produce one run;
- consent/provider changes after freeze do not change work; and
- Batch submission jobs contain only opaque durable identities.

### C. Execute durable Batch translation with leases and cancellation

**Depends on:** A and B.

Implement Batch workers, application fencing, typed retry
classification/backoff, cancellation guards, and the proactive reconciler.

Acceptance criteria:

- partial progress survives worker and process restart;
- cache-put-before-crash resumes as an exact hit;
- timeout, retry exhaustion, permanent failure, stale generation, lost lease,
  and cancellation semantics match this design;
- simultaneous runs respect the configured Batch request ceiling;
- job args, events, and stored errors pass privacy assertions; and
- Batch operation fails closed for endpoints without the accepted capability.

### D. Finalize exactly once through the atomic publication boundary

**Depends on:** A and B; integration with terminal outcomes from C.

Implement exact-cache materialization, manifest reconstruction, the finalizer,
and an atomic completion transaction that also finalizes the run.

Acceptance criteria:

- finalization reads only exact manifest keys and stored ordinal order;
- failed optional items yield accurate empty fields/completeness;
- injected failures publish no artifact, cards, or generated provenance;
- a retry after commit is a no-op success and a mismatched artifact fails
  immutable; and
- download remains a pure owner-scoped read with no partial APKG.

### E. Expose progress, metrics, stuck detection, and rollout controls

**Depends on:** B, C, and D.

Return internal phase/candidate progress through the existing status surface,
emit the required content-free telemetry, add stuck-run operational queries and
alerts, and document the accepted Batch configuration.

Acceptance criteria:

- progress is derived from outcome rows and is monotonic within a run;
- metrics/events contain no prohibited content or high-cardinality labels;
- stuck/expired work is detectable and reconciliation outcomes are visible;
- code-revert recovery does not strand committed Batch runs; and
- configured Batch limits are covered by end-to-end tests.

### F. Retire the legacy loop and keep approved cleanup policy

**Depends on:** successful canary of E and explicit retention approval.

The accepted standard-first cutover removes the in-memory prepared-deck translation loop
and keeps reconciliation durable. Keep
`internal/enrichmentjob` available for non-prepared bulk enrichment unless a
separate issue changes it. (The package was later removed: nothing submitted
bulk enrichment work after prepared decks owned translation.)

Acceptance criteria:

- every new preparation uses durable runs;
- no code can reselect a committed run or publish from a cancelled run;
- cleanup dry-run/audit counts and live-run/cache-reference exclusions remain
  verified before deletion; and
- operations and recovery documentation reflect the final configuration.

## Decision register

| Topic | Decision |
|---|---|
| Public identity | stable preparation ID plus immutable per-retry run ID |
| Manifest | owner-scoped, schema-versioned, insert-only, digest-verified |
| Cache | reuse the exact existing immutable key and first-writer behavior |
| Work unit | one manifest item/outcome, submitted in Batch chunks |
| Delivery | at-least-once provider calls, fenced idempotent persistence |
| Optional failure | terminal failed candidate; translation run may complete |
| Run failure | only an invariant that prevents trustworthy finalization |
| Concurrency | provider-managed Batch scheduling with a 5,000-request chunk ceiling |
| Finalization | one retriable finalizer and one atomic publication transaction |
| Cancellation | database state wins; River cancellation is best effort |
| Download | unchanged pure read of ready bytes |
| Content retention | unchanged initially; duration requires separate approval |

## Risks and open questions

- Duplicate billable provider calls remain possible across timeout/lease
  ambiguity. Exact-once external side effects are not achievable without a
  provider idempotency contract.
- Batch polling and provider-file expiry depend on durable timestamps and
  reconciliation health; stuck-run monitoring is required.
- Manifest rows intentionally duplicate private render inputs for durability,
  increasing database and backup size.
- The current cache key does not include tested target. This design preserves
  that accepted identity; any change requires a separate cache decision and
  migration rather than being folded into durable orchestration.
- Approval is still required for production attempt/backoff limits, global and
  provider-specific concurrency, retention duration, and any provider metadata
  beyond bounded error classes.
- The initial UI keeps one `preparing` state. A later UX issue may expose named
  freeze/translate/finalize labels without changing the public state machine.

## Related implementation

- `internal/prepareddeck/jobs.go`
- `internal/prepareddeck/jobs_integration_test.go`
- `internal/enrichment/enrichment.go`
- `internal/enrichmentjob/jobs.go` (since removed)
- `internal/cardexport/cardexport.go`
- `internal/persistence/deck_preparations.go`
- `migrations/000001_initialize.up.sql`
