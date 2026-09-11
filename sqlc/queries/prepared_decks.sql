-- Fenced prepared-deck transitions. The domain rules (claim-token validation,
-- expected-state WHERE guards, bounded-error validation, lost-claim rechecks)
-- stay in Go; these generated :one statements own the UPDATE ... RETURNING
-- composition and run inside the caller's transaction via WithTx.

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