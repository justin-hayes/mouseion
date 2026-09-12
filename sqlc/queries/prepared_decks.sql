-- Fenced prepared-deck transitions. The domain rules (claim-token validation,
-- expected-state WHERE guards, bounded-error validation, lost-claim rechecks)
-- stay in Go; these generated :one statements own the UPDATE ... RETURNING
-- composition and run inside the caller's transaction via WithTx.

-- name: StartPreparedDeckTranslation :exec
UPDATE deck_preparation_runs
SET translation_state = 'running', updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND translation_state = 'pending';

-- name: ClaimPreparedDeckTranslationOutcome :one
UPDATE deck_preparation_translation_outcomes o
SET state = 'running',
    dispatch_count = dispatch_count + 1,
    claim_token = $6,
    claimed_at = now(),
    lease_expires_at = $7,
    error_class = '',
    error_code = '',
    updated_at = now()
FROM deck_preparation_runs r
WHERE o.owner_id = $1
  AND o.preparation_id = $2
  AND o.run_id = $3
  AND o.ordinal = $4
  AND o.dispatch_generation = $5
  AND o.state = 'pending'
  AND o.next_attempt_at <= now()
  AND r.owner_id = o.owner_id
  AND r.preparation_id = o.preparation_id
  AND r.id = o.run_id
  AND r.state = 'translating'
  AND r.translation_state IN ('pending', 'running')
RETURNING o.owner_id, o.preparation_id, o.run_id, o.ordinal, o.state, o.dispatch_count, o.provider_attempt_count, o.max_provider_attempts, o.next_attempt_at, o.dispatch_generation, o.river_job_id, o.claim_token, o.claimed_at, o.lease_expires_at, o.terminal_at, o.error_class, o.error_code, o.cache_hit_count, o.provider_call_count, o.cache_latency_ms, o.provider_latency_ms, o.updated_at;

-- name: RetryPreparedDeckTranslationOutcome :one
UPDATE deck_preparation_translation_outcomes o
SET state = 'pending',
    provider_attempt_count = provider_attempt_count + 1,
    next_attempt_at = $7,
    dispatch_generation = dispatch_generation + 1,
    river_job_id = NULL,
    claim_token = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    error_class = $8,
    error_code = $9,
    updated_at = now()
FROM deck_preparation_runs r
WHERE o.owner_id = $1
  AND o.preparation_id = $2
  AND o.run_id = $3
  AND o.ordinal = $4
  AND o.dispatch_generation = $5
  AND o.claim_token = $6
  AND o.state = 'running'
  AND o.provider_attempt_count < o.max_provider_attempts
  AND r.owner_id = o.owner_id
  AND r.preparation_id = o.preparation_id
  AND r.id = o.run_id
  AND r.state = 'translating'
  AND r.translation_state IN ('pending', 'running')
RETURNING o.owner_id, o.preparation_id, o.run_id, o.ordinal, o.state, o.dispatch_count, o.provider_attempt_count, o.max_provider_attempts, o.next_attempt_at, o.dispatch_generation, o.river_job_id, o.claim_token, o.claimed_at, o.lease_expires_at, o.terminal_at, o.error_class, o.error_code, o.cache_hit_count, o.provider_call_count, o.cache_latency_ms, o.provider_latency_ms, o.updated_at;

-- name: RedispatchPreparedDeckTranslationOutcome :one
UPDATE deck_preparation_translation_outcomes o
SET state = 'pending',
    dispatch_generation = dispatch_generation + 1,
    river_job_id = NULL,
    claim_token = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    next_attempt_at = now(),
    error_class = 'orchestration',
    error_code = 'expired_lease',
    updated_at = now()
FROM deck_preparation_runs r
WHERE o.owner_id = $1
  AND o.preparation_id = $2
  AND o.run_id = $3
  AND o.ordinal = $4
  AND o.dispatch_generation = $5
  AND o.state = 'running'
  AND o.lease_expires_at <= now()
  AND r.owner_id = o.owner_id
  AND r.preparation_id = o.preparation_id
  AND r.id = o.run_id
  AND r.state = 'translating'
RETURNING o.owner_id, o.preparation_id, o.run_id, o.ordinal, o.state, o.dispatch_count, o.provider_attempt_count, o.max_provider_attempts, o.next_attempt_at, o.dispatch_generation, o.river_job_id, o.claim_token, o.claimed_at, o.lease_expires_at, o.terminal_at, o.error_class, o.error_code, o.cache_hit_count, o.provider_call_count, o.cache_latency_ms, o.provider_latency_ms, o.updated_at;

-- name: FinishPreparedDeckTranslationOutcome :one
UPDATE deck_preparation_translation_outcomes
SET state = $7,
    provider_attempt_count = provider_attempt_count + $8,
    claim_token = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    terminal_at = now(),
    error_class = $9,
    error_code = $10,
    cache_hit_count = cache_hit_count + $11,
    provider_call_count = provider_call_count + $12,
    cache_latency_ms = cache_latency_ms + $13,
    provider_latency_ms = provider_latency_ms + $14,
    updated_at = now()
WHERE owner_id = $1
  AND preparation_id = $2
  AND run_id = $3
  AND ordinal = $4
  AND dispatch_generation = $5
  AND claim_token = $6
  AND state = 'running'
RETURNING owner_id, preparation_id, run_id, ordinal, state, dispatch_count, provider_attempt_count, max_provider_attempts, next_attempt_at, dispatch_generation, river_job_id, claim_token, claimed_at, lease_expires_at, terminal_at, error_class, error_code, cache_hit_count, provider_call_count, cache_latency_ms, provider_latency_ms, updated_at;

-- name: ClaimPreparedDeckFinalization :one
UPDATE deck_preparation_runs r
SET finalization_claim_token = $5,
    finalization_claimed_at = now(),
    finalization_lease_expires_at = $6,
    finalization_dispatch_count = finalization_dispatch_count + 1,
    updated_at = now()
FROM deck_preparations p
WHERE r.owner_id = $1
  AND r.preparation_id = $2
  AND r.id = $3
  AND r.finalization_dispatch_generation = $4
  AND r.state = 'finalizing'
  AND r.translation_state = 'completed'
  AND (r.finalization_claim_token IS NULL OR r.finalization_lease_expires_at <= now())
  AND p.owner_id = r.owner_id
  AND p.id = r.preparation_id
  AND p.current_run_id = r.id
  AND p.state = 'preparing'
RETURNING r.id, r.owner_id, r.preparation_id, r.run_number, r.state, r.translation_state, r.execution_mode, r.target_language, r.external_translation_consent, r.external_translation_configured, r.context_mode, r.provider, r.provider_version, r.endpoint, r.model, r.manifest_schema_version, r.retry_policy_version, r.max_provider_attempts, r.max_batch_generations, r.batch_max_requests, r.batch_max_bytes, r.candidate_count, r.completed_count, r.failed_count, r.finalization_dispatch_generation, r.finalization_dispatch_count, r.finalization_job_id, r.finalization_claim_token, r.finalization_claimed_at, r.finalization_lease_expires_at, r.error_class, r.error_code, r.created_at, r.updated_at, r.translation_completed_at, r.completed_at;

-- name: AssignPreparedDeckFinalizationJob :one
UPDATE deck_preparation_runs
SET finalization_dispatch_generation = finalization_dispatch_generation + 1,
    finalization_job_id = $5,
    updated_at = now()
WHERE owner_id = $1
  AND preparation_id = $2
  AND id = $3
  AND finalization_dispatch_generation = $4
  AND state = 'finalizing'
  AND translation_state = 'completed'
RETURNING id, owner_id, preparation_id, run_number, state, translation_state, execution_mode, target_language, external_translation_consent, external_translation_configured, context_mode, provider, provider_version, endpoint, model, manifest_schema_version, retry_policy_version, max_provider_attempts, max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count, completed_count, failed_count, finalization_dispatch_generation, finalization_dispatch_count, finalization_job_id, finalization_claim_token, finalization_claimed_at, finalization_lease_expires_at, error_class, error_code, created_at, updated_at, translation_completed_at, completed_at;

-- name: VerifyPreparedDeckBatchSubmissionClaim :one
SELECT EXISTS(
  SELECT 1 FROM deck_preparation_batch_chunks c
  JOIN deck_preparation_runs r ON r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id
  WHERE c.owner_id = $1
    AND c.preparation_id = $2
    AND c.run_id = $3
    AND c.id = $4
    AND c.generation = $5
    AND c.submission_generation = $5
    AND c.submission_claim_token = $6
    AND c.state = 'submitting'
    AND r.state = 'translating'
);

-- name: PreparedDeckRunExists :one
SELECT EXISTS(
  SELECT 1 FROM deck_preparation_runs
  WHERE owner_id = sqlc.arg('owner') AND preparation_id = sqlc.arg('preparation') AND id = sqlc.arg('run')
);

-- name: GetPreparedDeckRunProgress :one
WITH outcomes AS (
  SELECT o.* FROM deck_preparation_translation_outcomes o
  WHERE o.owner_id = sqlc.arg('owner') AND o.preparation_id = sqlc.arg('preparation') AND o.run_id = sqlc.arg('run')
), manifest AS (
  SELECT mi.* FROM deck_preparation_manifest_items mi
  WHERE mi.owner_id = sqlc.arg('owner') AND mi.preparation_id = sqlc.arg('preparation') AND mi.run_id = sqlc.arg('run')
), chunks AS (
  SELECT c.* FROM deck_preparation_batch_chunks c
  WHERE c.owner_id = sqlc.arg('owner') AND c.preparation_id = sqlc.arg('preparation') AND c.run_id = sqlc.arg('run')
)
SELECT (SELECT count(*) FROM outcomes) AS candidate_count,
       (SELECT count(*) FROM outcomes WHERE state = 'pending') AS pending_count,
       (SELECT count(*) FROM outcomes WHERE state = 'running') AS running_count,
       (SELECT count(*) FROM outcomes WHERE state = 'completed') AS completed_count,
       (SELECT count(*) FROM outcomes WHERE state = 'failed') AS failed_count,
       (SELECT count(*) FROM outcomes WHERE state = 'cancelled') AS cancelled_count,
       (SELECT count(*) FROM outcomes WHERE state = 'pending' AND provider_attempt_count > 0) AS retrying_count,
       (SELECT count(*) FROM manifest WHERE disposition = 'quality_omitted') AS manifest_omissions,
       EXTRACT(EPOCH FROM COALESCE((SELECT min(submitted_at) FROM chunks WHERE submitted_at IS NOT NULL), 'epoch'::timestamptz))::double precision AS batch_submitted_epoch,
       (SELECT count(*) FROM chunks) AS batch_chunk_count,
       (SELECT count(*) FROM chunks WHERE state = 'submitted') AS batch_submitted_chunks,
       (SELECT count(*) FROM chunks WHERE state = 'polling') AS batch_polling_chunks,
       (SELECT count(*) FROM chunks WHERE state = 'reconciling') AS batch_reconciling_chunks,
       (SELECT count(*) FROM chunks WHERE state = 'completed') AS batch_completed_chunks,
       (SELECT count(*) FROM chunks WHERE state IN ('failed', 'ambiguous')) AS batch_failed_chunks,
       (SELECT count(*) FROM chunks WHERE state = 'cancelled') AS batch_cancelled_chunks,
       (SELECT COALESCE(sum(request_count), 0)::bigint FROM chunks) AS batch_request_count,
       (SELECT COALESCE(sum(completed_count), 0)::bigint FROM chunks) AS batch_completed_requests,
       (SELECT COALESCE(sum(failed_count), 0)::bigint FROM chunks) AS batch_failed_requests,
       (SELECT COALESCE(sum(expired_count), 0)::bigint FROM chunks) AS batch_expired_requests,
       (SELECT COALESCE(sum(input_tokens), 0)::bigint FROM chunks) AS batch_input_tokens,
       (SELECT COALESCE(sum(output_tokens), 0)::bigint FROM chunks) AS batch_output_tokens;

-- name: GetPreparedDeckTranslationCoverage :one
SELECT count(*) FILTER (WHERE COALESCE(ec.translation, '') <> '') AS cards_with_english,
       count(*) FILTER (WHERE COALESCE(ec.sentence_translation, '') <> '') AS cards_with_contextual_sentence_translations
FROM deck_preparation_manifest_items mi
JOIN deck_preparation_translation_outcomes o ON o.owner_id = mi.owner_id AND o.preparation_id = mi.preparation_id AND o.run_id = mi.run_id AND o.ordinal = mi.ordinal AND o.state = 'completed'
LEFT JOIN enrichment_cache ec ON ec.language = mi.language AND ec.target_language = mi.target_language AND ec.canonical_lemma = mi.canonical_lemma AND ec.upos = mi.upos AND ec.provider = mi.provider AND ec.provider_version = mi.provider_version AND ec.sentence_hash = COALESCE(mi.sentence_hash, '')
WHERE mi.owner_id = sqlc.arg('owner') AND mi.preparation_id = sqlc.arg('preparation') AND mi.run_id = sqlc.arg('run') AND mi.disposition = 'accepted';

-- name: ListPreparedDeckStuckBatches :many
SELECT owner_id::text, preparation_id::text, run_id::text, id::text, state,
       COALESCE(provider_status, ''), error_class, request_count,
       completed_count, failed_count, expired_count,
       EXTRACT(EPOCH FROM (now() - updated_at))::double precision AS age_seconds
FROM deck_preparation_batch_chunks
WHERE state IN ('pending', 'submitting', 'submitted', 'polling', 'reconciling')
  AND updated_at <= now() - (sqlc.arg('older_seconds')::double precision * interval '1 second')
ORDER BY updated_at ASC
LIMIT sqlc.arg('limit');

-- name: GetPreparedDeckRun :one
SELECT * FROM deck_preparation_runs WHERE owner_id = $1 AND preparation_id = $2 AND id = $3;

-- name: GetCurrentPreparedDeckRun :one
SELECT r.* FROM deck_preparation_runs r
WHERE r.owner_id = $1 AND r.preparation_id = $2 AND r.id = (SELECT p.current_run_id FROM deck_preparations p WHERE p.owner_id = $1 AND p.id = $2);

-- name: GetPreparedDeckRunForUpdate :one
SELECT * FROM deck_preparation_runs WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 FOR UPDATE;

-- name: CompletePreparedDeckRun :one
UPDATE deck_preparation_runs
SET state = 'completed', finalization_claim_token = NULL, finalization_claimed_at = NULL,
    finalization_lease_expires_at = NULL, error_class = '', error_code = '',
    completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3
  AND state = 'finalizing' AND finalization_claim_token = $4
RETURNING *;

-- name: FailPreparedDeckFinalizationRun :one
UPDATE deck_preparation_runs
SET state = 'failed', translation_state = 'failed', error_class = $5, error_code = $6,
    finalization_claim_token = NULL, finalization_claimed_at = NULL,
    finalization_lease_expires_at = NULL, completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3
  AND state = 'finalizing' AND finalization_claim_token = $4
RETURNING state;

-- name: GetPreparedDeckBatchChunkForUpdate :one
SELECT * FROM deck_preparation_batch_chunks WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 FOR UPDATE;

-- name: GetPreparedDeckTranslationOutcome :one
SELECT * FROM deck_preparation_translation_outcomes WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND ordinal = $4;

-- name: GetPreparedDeckTranslationOutcomeForUpdate :one
SELECT * FROM deck_preparation_translation_outcomes WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND ordinal = $4 FOR UPDATE;

-- name: ListPreparedDeckTranslationOutcomes :many
SELECT * FROM deck_preparation_translation_outcomes WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 ORDER BY ordinal;

-- name: ListPreparedDeckBatchChunks :many
SELECT * FROM deck_preparation_batch_chunks WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 ORDER BY generation, chunk_index;

-- name: ListPreparedDeckBatchChunkOrdinals :many
SELECT ordinal FROM deck_preparation_batch_chunk_items WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND chunk_id = $4 ORDER BY position;

-- name: ListPreparedDeckBatchChunkMembers :many
SELECT ci.ordinal, mi.language, mi.target_language, mi.canonical_lemma, mi.upos,
       COALESCE(mi.provider, ''), COALESCE(mi.provider_version, ''), COALESCE(mi.sentence_hash, '')
FROM deck_preparation_batch_chunk_items ci
JOIN deck_preparation_manifest_items mi ON mi.owner_id = ci.owner_id AND mi.preparation_id = ci.preparation_id AND mi.run_id = ci.run_id AND mi.ordinal = ci.ordinal
WHERE ci.owner_id = $1 AND ci.preparation_id = $2 AND ci.run_id = $3 AND ci.chunk_id = $4
ORDER BY ci.position;

-- name: AssignPreparedDeckBatchSubmissionJob :one
UPDATE deck_preparation_batch_chunks c
SET submission_generation = submission_generation + 1,
    submission_job_id = $6,
    updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.submission_generation = $5 AND c.state = 'pending'
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: GetDeckPreparationFreezeState :one
SELECT state, current_run_id, filename FROM deck_preparations WHERE owner_id = $1 AND id = $2 FOR UPDATE;

-- name: GetPreparedDeckManifest :one
SELECT * FROM deck_preparation_manifests WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3;

-- name: GetPreparedDeckManifestDigest :one
SELECT manifest_digest FROM deck_preparation_manifests WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3;

-- name: ListPreparedDeckManifestItems :many
SELECT * FROM deck_preparation_manifest_items WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 ORDER BY ordinal;

-- name: PreparedDeckCacheExists :one
SELECT EXISTS(
  SELECT 1 FROM enrichment_cache
  WHERE language = $1 AND target_language = $2 AND canonical_lemma = $3 AND upos = $4
    AND provider = $5 AND provider_version = $6 AND sentence_hash = $7
);

-- name: NextPreparedDeckRunNumber :one
SELECT COALESCE(max(run_number), 0) + 1 FROM deck_preparation_runs WHERE owner_id = $1 AND preparation_id = $2;

-- name: InsertPreparedDeckRun :exec
INSERT INTO deck_preparation_runs(id, owner_id, preparation_id, run_number, state, translation_state, execution_mode, target_language, external_translation_consent, external_translation_configured, context_mode, provider, provider_version, endpoint, model, manifest_schema_version, retry_policy_version, max_provider_attempts, max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count, completed_count, translation_completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24);

-- name: InsertPreparedDeckManifest :exec
INSERT INTO deck_preparation_manifests(owner_id, preparation_id, run_id, schema_version, manifest_digest, deck_name, filename, selected_count, accepted_count, omitted_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: InsertPreparedDeckManifestItem :exec
INSERT INTO deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal, disposition, language, target_language, canonical_lemma, upos, source_sentence, tested_target, first_encounter, quality_score, quality_reasons, render_payload, provider, provider_version, sentence_hash, candidate_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19);

-- name: InsertPreparedDeckTranslationOutcome :exec
INSERT INTO deck_preparation_translation_outcomes(owner_id, preparation_id, run_id, ordinal, state, max_provider_attempts, terminal_at, cache_hit_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertPreparedDeckBatchChunk :exec
INSERT INTO deck_preparation_batch_chunks(id, owner_id, preparation_id, run_id, chunk_index, generation, model, endpoint, split_reason, first_ordinal, last_ordinal, input_digest, request_count, input_bytes, estimated_prompt_tokens)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: InsertPreparedDeckBatchChunkItem :exec
INSERT INTO deck_preparation_batch_chunk_items(owner_id, preparation_id, run_id, chunk_id, generation, position, ordinal, candidate_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertPreparedDeckBatchChunkItemFromManifest :exec
INSERT INTO deck_preparation_batch_chunk_items(owner_id, preparation_id, run_id, chunk_id, generation, position, ordinal, candidate_digest)
SELECT sqlc.arg('owner')::uuid, sqlc.arg('preparation')::uuid, sqlc.arg('run')::uuid, sqlc.arg('chunk')::uuid, sqlc.arg('generation')::integer, sqlc.arg('position')::integer, sqlc.arg('ordinal')::integer, candidate_digest
FROM deck_preparation_manifest_items
WHERE owner_id = sqlc.arg('owner') AND preparation_id = sqlc.arg('preparation') AND run_id = sqlc.arg('run') AND ordinal = sqlc.arg('ordinal');

-- name: AttachPreparedDeckRun :exec
UPDATE deck_preparations
SET state = 'preparing', current_run_id = $3, started_at = COALESCE(started_at, now()),
    completed_at = NULL, error = '', updated_at = now()
WHERE owner_id = $1 AND id = $2;

-- name: SetPreparedDeckBatchSubmissionJob :execrows
UPDATE deck_preparation_batch_chunks
SET submission_job_id = $6, submission_generation = $5, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 AND generation = $5
  AND (state = 'pending' OR (state = 'submitting' AND submission_lease_expires_at <= now()));

-- name: SetPreparedDeckBatchSubmissionJobPending :execrows
UPDATE deck_preparation_batch_chunks
SET submission_job_id = $6, submission_generation = $5, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 AND generation = $5
  AND state = 'pending';

-- name: SetPreparedDeckTranslationJob :execrows
UPDATE deck_preparation_translation_outcomes
SET river_job_id = $6
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND ordinal = $4
  AND dispatch_generation = $5 AND state = 'pending' AND river_job_id IS NULL;

-- name: SetPreparedDeckFinalizationJob :execrows
UPDATE deck_preparation_runs
SET finalization_job_id = $5,
    finalization_claim_token = CASE WHEN finalization_claim_token IS NOT NULL AND finalization_lease_expires_at <= now() THEN NULL ELSE finalization_claim_token END,
    finalization_claimed_at = CASE WHEN finalization_claim_token IS NOT NULL AND finalization_lease_expires_at <= now() THEN NULL ELSE finalization_claimed_at END,
    finalization_lease_expires_at = CASE WHEN finalization_claim_token IS NOT NULL AND finalization_lease_expires_at <= now() THEN NULL ELSE finalization_lease_expires_at END,
    updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND finalization_dispatch_generation = $4 AND state = 'finalizing';

-- name: RecordPreparedDeckBatchSubmitted :one
UPDATE deck_preparation_batch_chunks c
SET state = 'submitted', input_file_id = $7, batch_id = $8, submitted_at = $9,
    submission_claim_token = NULL, submission_claimed_at = NULL, submission_lease_expires_at = NULL,
    error_class = '', error_code = '', updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.generation = $5 AND c.submission_generation = $5 AND c.submission_claim_token = $6 AND c.state = 'submitting'
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: AttachPreparedDeckBatchReconciliationJob :one
UPDATE deck_preparation_batch_chunks
SET reconciliation_job_id = $5, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 AND state = 'submitted' AND batch_id = $6
RETURNING *;

-- name: ClaimPreparedDeckBatchSubmission :one
UPDATE deck_preparation_batch_chunks c
SET state = 'submitting', submission_claim_token = $6, submission_claimed_at = now(), submission_lease_expires_at = $7, updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.submission_generation = $5 AND c.state IN ('pending', 'submitting')
  AND (c.submission_claim_token IS NULL OR c.submission_lease_expires_at <= now())
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: AssignPreparedDeckBatchReconciliationJob :one
UPDATE deck_preparation_batch_chunks c
SET reconciliation_generation = reconciliation_generation + 1,
    reconciliation_job_id = $6,
    updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.reconciliation_generation = $5 AND c.state IN ('submitted', 'polling')
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: ClaimPreparedDeckBatchReconciliation :one
UPDATE deck_preparation_batch_chunks c
SET state = 'reconciling', reconciliation_claim_token = $6, reconciliation_claimed_at = now(), reconciliation_lease_expires_at = $7, updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.reconciliation_generation = $5 AND c.state IN ('submitted', 'polling', 'reconciling')
  AND (c.reconciliation_claim_token IS NULL OR c.reconciliation_lease_expires_at <= now())
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: SetPreparedDeckBatchReconciliationJob :execrows
UPDATE deck_preparation_batch_chunks
SET reconciliation_job_id = $6,
    reconciliation_claim_token = CASE WHEN state = 'reconciling' AND reconciliation_lease_expires_at <= now() THEN NULL ELSE reconciliation_claim_token END,
    reconciliation_claimed_at = CASE WHEN state = 'reconciling' AND reconciliation_lease_expires_at <= now() THEN NULL ELSE reconciliation_claimed_at END,
    reconciliation_lease_expires_at = CASE WHEN state = 'reconciling' AND reconciliation_lease_expires_at <= now() THEN NULL ELSE reconciliation_lease_expires_at END,
    state = CASE WHEN state = 'reconciling' AND reconciliation_lease_expires_at <= now() THEN 'polling' ELSE state END,
    updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 AND reconciliation_generation = $5
  AND (state IN ('submitted', 'polling') OR (state = 'reconciling' AND reconciliation_lease_expires_at <= now()));

-- name: CompletePreparedDeckBatchCacheHits :one
WITH hit AS (
  UPDATE deck_preparation_translation_outcomes o
  SET state = 'completed', terminal_at = now(), cache_hit_count = cache_hit_count + 1, updated_at = now(),
      claim_token = NULL, claimed_at = NULL, lease_expires_at = NULL
  FROM deck_preparation_batch_chunk_items ci
  JOIN deck_preparation_batch_chunks c ON c.owner_id = ci.owner_id AND c.preparation_id = ci.preparation_id AND c.run_id = ci.run_id AND c.id = ci.chunk_id AND c.generation = ci.generation
  JOIN deck_preparation_manifest_items mi ON mi.owner_id = ci.owner_id AND mi.preparation_id = ci.preparation_id AND mi.run_id = ci.run_id AND mi.ordinal = ci.ordinal
  JOIN enrichment_cache ec ON ec.language = mi.language AND ec.target_language = mi.target_language AND ec.canonical_lemma = mi.canonical_lemma AND ec.upos = mi.upos AND ec.provider = mi.provider AND ec.provider_version = mi.provider_version AND ec.sentence_hash = COALESCE(mi.sentence_hash, '')
  WHERE o.owner_id = $1 AND o.preparation_id = $2 AND o.run_id = $3 AND o.ordinal = ci.ordinal AND o.state = 'pending'
    AND c.id = $4 AND c.generation = $5 AND c.state = 'submitting' AND c.submission_claim_token = $6
  RETURNING o.ordinal
) SELECT count(*) FROM hit;

-- name: FinishPreparedDeckBatchSubmission :one
UPDATE deck_preparation_batch_chunks c
SET state = $7, error_class = $8, error_code = $9,
    completed_count = CASE WHEN $7 = 'completed' THEN request_count ELSE completed_count END,
    submission_claim_token = NULL, submission_claimed_at = NULL, submission_lease_expires_at = NULL, updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.generation = $5 AND c.submission_claim_token = $6 AND c.state = 'submitting'
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: RetryPreparedDeckBatchSubmission :one
UPDATE deck_preparation_batch_chunks c
SET state = 'pending', error_class = $7, error_code = $8,
    submission_claim_token = NULL, submission_claimed_at = NULL, submission_lease_expires_at = NULL, updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = $1 AND c.preparation_id = $2 AND c.run_id = $3 AND c.id = $4
  AND c.generation = $5 AND c.submission_claim_token = $6 AND c.state = 'submitting'
  AND c.input_file_id IS NULL AND c.batch_id IS NULL
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: FinishPreparedDeckBatchReconciliation :one
UPDATE deck_preparation_batch_chunks c
SET state = sqlc.arg('state'), provider_status = NULLIF(sqlc.arg('provider_status')::text, ''), output_file_id = NULLIF(sqlc.arg('output_file_id')::text, ''), error_file_id = NULLIF(sqlc.arg('error_file_id')::text, ''),
    completed_count = sqlc.arg('completed_count'), failed_count = sqlc.arg('failed_count'), expired_count = sqlc.arg('expired_count'), input_tokens = sqlc.arg('input_tokens'), output_tokens = sqlc.arg('output_tokens'), total_tokens = sqlc.arg('input_tokens')::bigint + sqlc.arg('output_tokens')::bigint,
    error_class = sqlc.arg('error_class'), error_code = sqlc.arg('error_code'), provider_completed_at = sqlc.arg('provider_completed_at'), last_polled_at = now(),
    reconciled_at = CASE WHEN sqlc.arg('state') IN ('completed', 'failed') THEN now() ELSE reconciled_at END,
    reconciliation_claim_token = NULL, reconciliation_claimed_at = NULL, reconciliation_lease_expires_at = NULL, updated_at = now()
FROM deck_preparation_runs r
WHERE c.owner_id = sqlc.arg('owner') AND c.preparation_id = sqlc.arg('preparation') AND c.run_id = sqlc.arg('run') AND c.id = sqlc.arg('id')
  AND c.reconciliation_generation = sqlc.arg('generation') AND c.reconciliation_claim_token = sqlc.arg('token') AND c.state = 'reconciling'
  AND r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id AND r.state = 'translating'
RETURNING c.*;

-- name: CompletePreparedDeckBatchChunk :one
UPDATE deck_preparation_batch_chunks
SET state = 'completed', provider_status = sqlc.arg('provider_status'), output_file_id = NULLIF(sqlc.arg('output_file_id')::text, ''), error_file_id = NULLIF(sqlc.arg('error_file_id')::text, ''),
    completed_count = sqlc.arg('completed_count'), failed_count = sqlc.arg('failed_count'), expired_count = sqlc.arg('expired_count'), input_tokens = sqlc.arg('input_tokens'), output_tokens = sqlc.arg('output_tokens'), total_tokens = sqlc.arg('input_tokens')::bigint + sqlc.arg('output_tokens')::bigint,
    error_class = sqlc.arg('error_class'), error_code = sqlc.arg('error_code'), provider_completed_at = sqlc.arg('provider_completed_at'), reconciled_at = now(), last_polled_at = now(),
    reconciliation_claim_token = NULL, reconciliation_claimed_at = NULL, reconciliation_lease_expires_at = NULL, updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND preparation_id = sqlc.arg('preparation') AND run_id = sqlc.arg('run') AND id = sqlc.arg('id')
RETURNING *;

-- name: UpsertEnrichmentCache :exec
INSERT INTO enrichment_cache(language, target_language, canonical_lemma, upos, provider, provider_version, sentence_hash, translation, gloss, sentence_translation, sentence_translation_target, cached_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT DO NOTHING;

-- name: EnrichmentCacheLookup :one
SELECT 1 FROM enrichment_cache
WHERE language = $1 AND target_language = $2 AND canonical_lemma = $3 AND upos = $4
  AND provider = $5 AND provider_version = $6 AND sentence_hash = $7;

-- name: UpdatePreparedDeckOutcomeFromBatch :exec
UPDATE deck_preparation_translation_outcomes
SET state = $5, provider_attempt_count = GREATEST(provider_attempt_count, $6), next_attempt_at = now(),
    claim_token = NULL, claimed_at = NULL, lease_expires_at = NULL, terminal_at = $7,
    error_class = $8, error_code = $9, provider_call_count = provider_call_count + 1, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND ordinal = $4;

-- name: NextPreparedDeckBatchChunkIndex :one
SELECT COALESCE(max(chunk_index), -1) + 1 FROM deck_preparation_batch_chunks
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND generation = $4;

-- name: CountPreparedDeckRunOutcomeStates :one
SELECT count(*) FILTER (WHERE state = 'completed') AS completed_count,
       count(*) FILTER (WHERE state = 'failed') AS failed_count,
       count(*) FILTER (WHERE state IN ('pending', 'running')) AS nonterminal_count
FROM deck_preparation_translation_outcomes
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3;

-- name: UpdatePreparedDeckRunTranslationRunning :one
UPDATE deck_preparation_runs
SET translation_state = 'running', completed_count = $4, failed_count = $5, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state = 'translating'
RETURNING *;

-- name: FailPreparedDeckRunIncomplete :one
UPDATE deck_preparation_runs
SET state = 'failed', translation_state = 'failed', completed_count = $4, failed_count = $5,
    error_class = 'translation', error_code = 'incomplete', completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state = 'translating'
RETURNING *;

-- name: FinalizePreparedDeckRun :one
UPDATE deck_preparation_runs
SET state = 'finalizing', translation_state = 'completed', completed_count = $4, failed_count = $5,
    translation_completed_at = COALESCE(translation_completed_at, now()), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state = 'translating'
RETURNING *;

-- name: FailDeckPreparationTranslation :execrows
UPDATE deck_preparations
SET state = 'failed', error = $4, completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND id = $2 AND current_run_id = $3 AND state = 'preparing';

-- name: GetDeckPreparationStateForRun :one
SELECT state FROM deck_preparations WHERE owner_id = $1 AND id = $2 AND current_run_id = $3 FOR UPDATE;

-- name: LockPreparedDeckRunTranslating :one
SELECT 1 FROM deck_preparation_runs
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state = 'translating'
FOR UPDATE;

-- name: FailPreparedDeckBatchChunk :execrows
UPDATE deck_preparation_batch_chunks
SET state = 'failed', provider_status = $7, error_class = $8, error_code = $9,
    reconciled_at = now(), last_polled_at = now(),
    reconciliation_claim_token = NULL, reconciliation_claimed_at = NULL, reconciliation_lease_expires_at = NULL, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4
  AND reconciliation_generation = $5 AND reconciliation_claim_token = $6 AND state = 'reconciling';

-- name: FailPreparedDeckBatchChunkSubmission :execrows
UPDATE deck_preparation_batch_chunks
SET state = $7, error_class = $8, error_code = $9,
    submission_claim_token = NULL, submission_claimed_at = NULL, submission_lease_expires_at = NULL, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4
  AND generation = $5 AND submission_claim_token = $6 AND state = 'submitting';

-- name: FailPreparedDeckRun :execrows
UPDATE deck_preparation_runs
SET state = 'failed', translation_state = 'failed', error_class = $4, error_code = $5,
    completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state = 'translating';

-- name: CancelPreparedDeckRun :exec
UPDATE deck_preparation_runs
SET state = 'cancelled',
    translation_state = CASE WHEN translation_state IN ('pending', 'running') THEN 'cancelled' ELSE translation_state END,
    finalization_claim_token = NULL, finalization_claimed_at = NULL, finalization_lease_expires_at = NULL,
    error_class = '', error_code = '', completed_at = now(), updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND id = $3 AND state IN ('translating', 'finalizing');

-- name: CancelPreparedDeckOutcomes :exec
UPDATE deck_preparation_translation_outcomes
SET state = 'cancelled', claim_token = NULL, claimed_at = NULL, lease_expires_at = NULL,
    terminal_at = now(), error_class = 'cancellation', error_code = '', updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND state IN ('pending', 'running');

-- name: CancelPreparedDeckBatchChunks :exec
UPDATE deck_preparation_batch_chunks
SET state = 'cancelled',
    submission_claim_token = NULL, submission_claimed_at = NULL, submission_lease_expires_at = NULL,
    reconciliation_claim_token = NULL, reconciliation_claimed_at = NULL, reconciliation_lease_expires_at = NULL,
    error_class = 'cancelled', error_code = '', updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND state NOT IN ('completed', 'failed', 'cancelled');

-- name: ListPreparedDeckLiveBatchIDs :many
SELECT batch_id FROM deck_preparation_batch_chunks
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND batch_id IS NOT NULL
  AND COALESCE(provider_status, '') NOT IN ('completed', 'failed', 'expired', 'cancelled')
ORDER BY generation, chunk_index;

-- name: ClaimPreparedDeckBatchCleanup :one
UPDATE deck_preparation_batch_chunks
SET cleanup_claim_token = $5, cleanup_claimed_at = now(), cleanup_lease_expires_at = $6, updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4
  AND state IN ('completed', 'cancelled')
  AND (cleanup_claim_token IS NULL OR cleanup_lease_expires_at <= now())
  AND ((input_file_id IS NOT NULL AND input_file_cleanup_state IN ('pending', 'failed') AND input_file_cleanup_attempts < $7) OR
       (output_file_id IS NOT NULL AND output_file_cleanup_state IN ('pending', 'failed') AND output_file_cleanup_attempts < $7) OR
       (error_file_id IS NOT NULL AND error_file_cleanup_state IN ('pending', 'failed') AND error_file_cleanup_attempts < $7))
RETURNING *;

-- name: FinishPreparedDeckBatchCleanup :one
UPDATE deck_preparation_batch_chunks
SET input_file_cleanup_state = $6, output_file_cleanup_state = $7, error_file_cleanup_state = $8,
    input_file_cleanup_attempts = $9, output_file_cleanup_attempts = $10, error_file_cleanup_attempts = $11,
    cleanup_error_class = $12, cleanup_error_code = $13,
    cleanup_claim_token = NULL, cleanup_claimed_at = NULL, cleanup_lease_expires_at = NULL,
    cleanup_completed_at = CASE WHEN $6 IN ('deleted', 'not_needed') AND $7 IN ('deleted', 'not_needed') AND $8 IN ('deleted', 'not_needed') THEN COALESCE(cleanup_completed_at, now()) ELSE NULL END,
    updated_at = now()
WHERE owner_id = $1 AND preparation_id = $2 AND run_id = $3 AND id = $4 AND cleanup_claim_token = $5
  AND state IN ('completed', 'cancelled')
RETURNING *;

-- name: ListPreparedDeckRecoveryWork :many
SELECT o.owner_id::text, o.preparation_id::text, o.run_id::text, ''::text AS chunk_id, o.ordinal,
       o.dispatch_generation AS generation, 'outcome'::text AS kind, o.state = 'running' AS lease_expired
FROM deck_preparation_translation_outcomes o
JOIN deck_preparation_runs r ON r.owner_id = o.owner_id AND r.preparation_id = o.preparation_id AND r.id = o.run_id
WHERE r.state = 'translating' AND ((o.state = 'pending' AND o.next_attempt_at <= now()) OR (o.state = 'running' AND o.lease_expires_at <= now()))
UNION ALL
SELECT c.owner_id::text, c.preparation_id::text, c.run_id::text, c.id::text, -1,
       c.submission_generation, 'batch_submission'::text, c.state = 'submitting'
FROM deck_preparation_batch_chunks c
JOIN deck_preparation_runs r ON r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id
WHERE r.state = 'translating' AND (c.state = 'pending' OR (c.state = 'submitting' AND c.submission_lease_expires_at <= now()))
UNION ALL
SELECT c.owner_id::text, c.preparation_id::text, c.run_id::text, c.id::text, -1,
       c.reconciliation_generation, 'batch_reconciliation'::text, c.state = 'reconciling'
FROM deck_preparation_batch_chunks c
JOIN deck_preparation_runs r ON r.owner_id = c.owner_id AND r.preparation_id = c.preparation_id AND r.id = c.run_id
WHERE r.state = 'translating' AND (c.state IN ('submitted', 'polling') OR (c.state = 'reconciling' AND c.reconciliation_lease_expires_at <= now()))
UNION ALL
SELECT owner_id::text, preparation_id::text, id::text, ''::text, -1,
       finalization_dispatch_generation, 'finalizer'::text, finalization_claim_token IS NOT NULL AND finalization_lease_expires_at <= now()
FROM deck_preparation_runs
WHERE state = 'finalizing' AND translation_state = 'completed' AND (finalization_claim_token IS NULL OR finalization_lease_expires_at <= now())
UNION ALL
SELECT owner_id::text, preparation_id::text, run_id::text, id::text, -1,
       generation, 'batch_cleanup'::text, cleanup_claim_token IS NOT NULL AND cleanup_lease_expires_at <= now()
FROM deck_preparation_batch_chunks
WHERE state IN ('completed', 'cancelled') AND (cleanup_claim_token IS NULL OR cleanup_lease_expires_at <= now()) AND
      ((input_file_id IS NOT NULL AND input_file_cleanup_state IN ('pending', 'failed') AND input_file_cleanup_attempts < 3) OR
       (output_file_id IS NOT NULL AND output_file_cleanup_state IN ('pending', 'failed') AND output_file_cleanup_attempts < 3) OR
       (error_file_id IS NOT NULL AND error_file_cleanup_state IN ('pending', 'failed') AND error_file_cleanup_attempts < 3))
UNION ALL
SELECT owner_id::text, preparation_id::text, id::text, ''::text, -1,
       finalization_dispatch_generation, 'translation_completion'::text, false
FROM deck_preparation_runs r
WHERE state = 'translating' AND NOT EXISTS (SELECT 1 FROM deck_preparation_translation_outcomes o WHERE o.run_id = r.id AND o.state IN ('pending', 'running'))
ORDER BY 7, 3, 5
LIMIT $1;
