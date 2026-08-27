CREATE TABLE deck_preparation_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_number integer NOT NULL CHECK(run_number > 0),
 state text NOT NULL CHECK(state IN ('translating','finalizing','completed','failed','cancelled')),
 translation_state text NOT NULL CHECK(translation_state IN ('pending','running','completed','failed','cancelled')),
 external_translation_consent boolean NOT NULL,
 external_translation_configured boolean NOT NULL,
 context_mode text,
 provider text,
 provider_version text,
 endpoint text,
 model text,
 manifest_schema_version integer NOT NULL CHECK(manifest_schema_version > 0),
 retry_policy_version integer NOT NULL CHECK(retry_policy_version > 0),
 max_provider_attempts integer NOT NULL CHECK(max_provider_attempts > 0),
 max_batch_generations integer NOT NULL CHECK(max_batch_generations > 0),
 batch_max_requests integer NOT NULL CHECK(batch_max_requests > 0 AND batch_max_requests <= 50000),
 batch_max_bytes bigint NOT NULL CHECK(batch_max_bytes > 0 AND batch_max_bytes <= 209715200),
 candidate_count integer NOT NULL CHECK(candidate_count >= 0),
 completed_count integer NOT NULL DEFAULT 0 CHECK(completed_count >= 0),
 failed_count integer NOT NULL DEFAULT 0 CHECK(failed_count >= 0),
 finalization_dispatch_generation integer NOT NULL DEFAULT 0 CHECK(finalization_dispatch_generation >= 0),
 finalization_dispatch_count integer NOT NULL DEFAULT 0 CHECK(finalization_dispatch_count >= 0),
 finalization_job_id bigint,
 finalization_claim_token uuid,
 finalization_claimed_at timestamptz,
 finalization_lease_expires_at timestamptz,
 error_class text NOT NULL DEFAULT '' CHECK(error_class IN ('','configuration','identity','orchestration','persistence','render','cache','provider','validation','retry_exhausted','ambiguous_submission','reconciliation')),
 error_code text NOT NULL DEFAULT '' CHECK(error_code ~ '^[a-z0-9_.:-]{0,80}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 translation_completed_at timestamptz,
 completed_at timestamptz,
 UNIQUE(owner_id, preparation_id, run_number),
 UNIQUE(owner_id, preparation_id, id),
 FOREIGN KEY(owner_id, preparation_id) REFERENCES deck_preparations(owner_id, id) ON DELETE CASCADE,
 CHECK(completed_count + failed_count <= candidate_count),
 CHECK((external_translation_consent AND external_translation_configured AND
        context_mode IS NOT NULL AND context_mode IN ('sentence','lemma_only') AND
        provider IS NOT NULL AND provider <> '' AND provider_version IS NOT NULL AND provider_version <> '' AND
        endpoint IS NOT NULL AND endpoint = '/v1/chat/completions' AND model IS NOT NULL AND model <> '') OR
       (NOT (external_translation_consent AND external_translation_configured) AND
        context_mode IS NULL AND provider IS NULL AND provider_version IS NULL AND endpoint IS NULL AND model IS NULL)),
 CHECK((finalization_claim_token IS NULL AND finalization_claimed_at IS NULL AND finalization_lease_expires_at IS NULL) OR
       (finalization_claim_token IS NOT NULL AND finalization_claimed_at IS NOT NULL AND finalization_lease_expires_at > finalization_claimed_at)),
 CHECK((state IN ('completed','failed','cancelled') AND completed_at IS NOT NULL) OR
       (state IN ('translating','finalizing') AND completed_at IS NULL)),
 CHECK((translation_state = 'completed' AND translation_completed_at IS NOT NULL) OR
       (translation_state <> 'completed'))
);

CREATE TABLE deck_preparation_manifests (
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_id uuid PRIMARY KEY,
 schema_version integer NOT NULL CHECK(schema_version > 0),
 manifest_digest text NOT NULL CHECK(manifest_digest ~ '^[0-9a-f]{64}$'),
 deck_name text NOT NULL CHECK(deck_name <> ''),
 filename text NOT NULL CHECK(filename <> ''),
 selected_count integer NOT NULL CHECK(selected_count >= 0),
 accepted_count integer NOT NULL CHECK(accepted_count >= 0),
 omitted_count integer NOT NULL CHECK(omitted_count >= 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, preparation_id, run_id),
 FOREIGN KEY(owner_id, preparation_id, run_id)
  REFERENCES deck_preparation_runs(owner_id, preparation_id, id) ON DELETE CASCADE,
 CHECK(selected_count = accepted_count + omitted_count)
);

CREATE TABLE deck_preparation_manifest_items (
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_id uuid NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal >= 0),
 disposition text NOT NULL CHECK(disposition IN ('accepted','quality_omitted')),
 language text NOT NULL CHECK(language <> ''),
 canonical_lemma text NOT NULL CHECK(canonical_lemma <> ''),
 upos text NOT NULL CHECK(upos <> '' AND upos = upper(upos)),
 source_sentence text NOT NULL,
 tested_target text NOT NULL,
 first_encounter bigint NOT NULL,
 quality_score integer NOT NULL CHECK(quality_score BETWEEN 0 AND 110),
 quality_reasons text[] NOT NULL DEFAULT '{}',
 render_payload jsonb NOT NULL CHECK(jsonb_typeof(render_payload) = 'object'),
 provider text,
 provider_version text,
 sentence_hash text,
 candidate_digest text NOT NULL CHECK(candidate_digest ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(run_id, ordinal),
 UNIQUE(run_id, candidate_digest),
 UNIQUE(owner_id, preparation_id, run_id, ordinal),
 UNIQUE(owner_id, preparation_id, run_id, ordinal, candidate_digest),
 FOREIGN KEY(owner_id, preparation_id, run_id)
  REFERENCES deck_preparation_manifests(owner_id, preparation_id, run_id) ON DELETE CASCADE,
 CHECK(cardinality(quality_reasons) <= 16 AND array_position(quality_reasons, NULL) IS NULL AND
       quality_reasons <@ ARRAY['too short or fragmented','too long','usable length','useful context window','target not present as a word','target present','invalid source location','valid source location','complete sentence boundaries','incomplete sentence boundaries','structural noise or boilerplate','no obvious structural noise']::text[]),
 CHECK((provider IS NULL AND provider_version IS NULL AND sentence_hash IS NULL) OR
       (provider IS NOT NULL AND provider <> '' AND provider_version IS NOT NULL AND provider_version <> '' AND
        sentence_hash IS NOT NULL AND (sentence_hash = '' OR sentence_hash ~ '^[0-9a-f]{64}$'))),
 CHECK((disposition = 'accepted') OR
       (disposition = 'quality_omitted' AND provider IS NULL AND provider_version IS NULL AND sentence_hash IS NULL))
);

CREATE TABLE deck_preparation_translation_outcomes (
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_id uuid NOT NULL,
 ordinal integer NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','running','completed','failed','cancelled')),
 dispatch_count integer NOT NULL DEFAULT 0 CHECK(dispatch_count >= 0),
 provider_attempt_count integer NOT NULL DEFAULT 0 CHECK(provider_attempt_count >= 0),
 max_provider_attempts integer NOT NULL CHECK(max_provider_attempts > 0),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 dispatch_generation integer NOT NULL DEFAULT 0 CHECK(dispatch_generation >= 0),
 river_job_id bigint,
 claim_token uuid,
 claimed_at timestamptz,
 lease_expires_at timestamptz,
 terminal_at timestamptz,
 error_class text NOT NULL DEFAULT '' CHECK(error_class IN ('','cache','provider','rate_limit','provider_5xx','timeout','cancellation','validation','identity','retry_exhausted','malformed_result','missing_result','duplicate_result','unknown_result','expired','orchestration')),
 error_code text NOT NULL DEFAULT '' CHECK(error_code ~ '^[a-z0-9_.:-]{0,80}$'),
 cache_hit_count integer NOT NULL DEFAULT 0 CHECK(cache_hit_count >= 0),
 provider_call_count integer NOT NULL DEFAULT 0 CHECK(provider_call_count >= 0),
 cache_latency_ms bigint NOT NULL DEFAULT 0 CHECK(cache_latency_ms >= 0),
 provider_latency_ms bigint NOT NULL DEFAULT 0 CHECK(provider_latency_ms >= 0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(run_id, ordinal),
 UNIQUE(owner_id, preparation_id, run_id, ordinal),
 FOREIGN KEY(owner_id, preparation_id, run_id, ordinal)
  REFERENCES deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal) ON DELETE CASCADE,
 CHECK(provider_attempt_count <= max_provider_attempts),
 CHECK((state = 'running' AND claim_token IS NOT NULL AND claimed_at IS NOT NULL AND lease_expires_at > claimed_at AND terminal_at IS NULL) OR
       (state = 'pending' AND claim_token IS NULL AND claimed_at IS NULL AND lease_expires_at IS NULL AND terminal_at IS NULL) OR
       (state IN ('completed','failed','cancelled') AND claim_token IS NULL AND claimed_at IS NULL AND lease_expires_at IS NULL AND terminal_at IS NOT NULL))
);

CREATE TABLE deck_preparation_batch_chunks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_id uuid NOT NULL,
 chunk_index integer NOT NULL CHECK(chunk_index >= 0),
 generation integer NOT NULL CHECK(generation > 0),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','submitting','submitted','polling','reconciling','completed','failed','cancelled','ambiguous')),
 provider_status text CHECK(provider_status IS NULL OR provider_status IN ('validating','in_progress','finalizing','completed','failed','expired','cancelling','cancelled')),
 model text NOT NULL CHECK(model <> ''),
 endpoint text NOT NULL CHECK(endpoint = '/v1/chat/completions'),
 split_reason text NOT NULL CHECK(split_reason IN ('run','request_limit','byte_limit','token_limit','retry')),
 first_ordinal integer NOT NULL CHECK(first_ordinal >= 0),
 last_ordinal integer NOT NULL CHECK(last_ordinal >= first_ordinal),
 input_digest text NOT NULL CHECK(input_digest ~ '^[0-9a-f]{64}$'),
 request_count integer NOT NULL CHECK(request_count > 0 AND request_count <= 50000),
 input_bytes bigint NOT NULL CHECK(input_bytes > 0 AND input_bytes <= 209715200),
 estimated_prompt_tokens bigint NOT NULL CHECK(estimated_prompt_tokens >= 0),
 completed_count integer NOT NULL DEFAULT 0 CHECK(completed_count >= 0),
 failed_count integer NOT NULL DEFAULT 0 CHECK(failed_count >= 0),
 expired_count integer NOT NULL DEFAULT 0 CHECK(expired_count >= 0),
 input_file_id text CHECK(input_file_id IS NULL OR input_file_id <> ''),
 batch_id text CHECK(batch_id IS NULL OR batch_id <> ''),
 output_file_id text CHECK(output_file_id IS NULL OR output_file_id <> ''),
 error_file_id text CHECK(error_file_id IS NULL OR error_file_id <> ''),
 submission_job_id bigint,
 submission_generation integer NOT NULL DEFAULT 0 CHECK(submission_generation >= 0),
 submission_claim_token uuid,
 submission_claimed_at timestamptz,
 submission_lease_expires_at timestamptz,
 reconciliation_job_id bigint,
 reconciliation_generation integer NOT NULL DEFAULT 0 CHECK(reconciliation_generation >= 0),
 reconciliation_claim_token uuid,
 reconciliation_claimed_at timestamptz,
 reconciliation_lease_expires_at timestamptz,
 error_class text NOT NULL DEFAULT '' CHECK(error_class IN ('','configuration','upload','submission','ambiguous_submission','provider','unsupported_model','expired','cancelled','poll','reconciliation','malformed_result','missing_result','duplicate_result','unknown_result','validation')),
 error_code text NOT NULL DEFAULT '' CHECK(error_code ~ '^[a-z0-9_.:-]{0,80}$'),
 input_tokens bigint NOT NULL DEFAULT 0 CHECK(input_tokens >= 0),
 output_tokens bigint NOT NULL DEFAULT 0 CHECK(output_tokens >= 0),
 total_tokens bigint NOT NULL DEFAULT 0 CHECK(total_tokens >= 0 AND total_tokens = input_tokens + output_tokens),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 submitted_at timestamptz,
 last_polled_at timestamptz,
 provider_completed_at timestamptz,
 reconciled_at timestamptz,
 UNIQUE(owner_id, preparation_id, run_id, id),
 UNIQUE(owner_id, preparation_id, run_id, id, generation),
 UNIQUE(run_id, generation, chunk_index),
 FOREIGN KEY(owner_id, preparation_id, run_id)
  REFERENCES deck_preparation_runs(owner_id, preparation_id, id) ON DELETE CASCADE,
 CHECK(completed_count + failed_count + expired_count <= request_count),
 CHECK((submission_claim_token IS NULL AND submission_claimed_at IS NULL AND submission_lease_expires_at IS NULL) OR
       (submission_claim_token IS NOT NULL AND submission_claimed_at IS NOT NULL AND submission_lease_expires_at > submission_claimed_at)),
 CHECK((reconciliation_claim_token IS NULL AND reconciliation_claimed_at IS NULL AND reconciliation_lease_expires_at IS NULL) OR
       (reconciliation_claim_token IS NOT NULL AND reconciliation_claimed_at IS NOT NULL AND reconciliation_lease_expires_at > reconciliation_claimed_at))
);

CREATE TABLE deck_preparation_batch_chunk_items (
 owner_id uuid NOT NULL,
 preparation_id uuid NOT NULL,
 run_id uuid NOT NULL,
 chunk_id uuid NOT NULL,
 generation integer NOT NULL CHECK(generation > 0),
 position integer NOT NULL CHECK(position >= 0),
 ordinal integer NOT NULL CHECK(ordinal >= 0),
 candidate_digest text NOT NULL CHECK(candidate_digest ~ '^[0-9a-f]{64}$'),
 PRIMARY KEY(chunk_id, position),
 UNIQUE(chunk_id, ordinal),
 UNIQUE(run_id, generation, ordinal),
 FOREIGN KEY(owner_id, preparation_id, run_id, chunk_id, generation)
  REFERENCES deck_preparation_batch_chunks(owner_id, preparation_id, run_id, id, generation) ON DELETE CASCADE,
 FOREIGN KEY(owner_id, preparation_id, run_id, ordinal, candidate_digest)
  REFERENCES deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal, candidate_digest) ON DELETE CASCADE
);

ALTER TABLE deck_preparations ADD COLUMN current_run_id uuid;
ALTER TABLE deck_preparations ADD CONSTRAINT deck_preparations_current_run_fkey
 FOREIGN KEY(owner_id, id, current_run_id)
 REFERENCES deck_preparation_runs(owner_id, preparation_id, id);

CREATE INDEX deck_preparation_runs_owner_history_idx ON deck_preparation_runs(owner_id, preparation_id, run_number DESC);
CREATE INDEX deck_preparation_runs_stuck_idx ON deck_preparation_runs(state, updated_at) WHERE state IN ('translating','finalizing');
CREATE INDEX deck_preparation_runs_finalizer_lease_idx ON deck_preparation_runs(finalization_lease_expires_at) WHERE finalization_claim_token IS NOT NULL;
CREATE INDEX deck_preparation_outcomes_pending_idx ON deck_preparation_translation_outcomes(next_attempt_at) WHERE state = 'pending';
CREATE INDEX deck_preparation_outcomes_running_lease_idx ON deck_preparation_translation_outcomes(lease_expires_at) WHERE state = 'running';
CREATE INDEX deck_preparation_outcomes_run_state_idx ON deck_preparation_translation_outcomes(run_id, state);
CREATE UNIQUE INDEX deck_preparation_outcomes_river_job_idx ON deck_preparation_translation_outcomes(river_job_id) WHERE river_job_id IS NOT NULL;
CREATE INDEX deck_preparation_batch_chunks_run_state_idx ON deck_preparation_batch_chunks(run_id, state, generation, chunk_index);
CREATE INDEX deck_preparation_batch_chunks_submission_lease_idx ON deck_preparation_batch_chunks(submission_lease_expires_at) WHERE submission_claim_token IS NOT NULL;
CREATE INDEX deck_preparation_batch_chunks_reconciliation_lease_idx ON deck_preparation_batch_chunks(reconciliation_lease_expires_at) WHERE reconciliation_claim_token IS NOT NULL;
CREATE UNIQUE INDEX deck_preparation_batch_chunks_batch_id_idx ON deck_preparation_batch_chunks(batch_id) WHERE batch_id IS NOT NULL;
CREATE UNIQUE INDEX deck_preparation_batch_chunks_input_file_id_idx ON deck_preparation_batch_chunks(input_file_id) WHERE input_file_id IS NOT NULL;

CREATE FUNCTION reject_deck_preparation_manifest_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'prepared-deck manifests are immutable';
END;
$$;
CREATE TRIGGER deck_preparation_manifests_immutable
 BEFORE UPDATE ON deck_preparation_manifests FOR EACH ROW
 EXECUTE FUNCTION reject_deck_preparation_manifest_mutation();
CREATE TRIGGER deck_preparation_manifest_items_immutable
 BEFORE UPDATE ON deck_preparation_manifest_items FOR EACH ROW
 EXECUTE FUNCTION reject_deck_preparation_manifest_mutation();

CREATE FUNCTION reject_deck_preparation_batch_chunk_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.owner_id <> OLD.owner_id OR NEW.preparation_id <> OLD.preparation_id OR
    NEW.run_id <> OLD.run_id OR NEW.chunk_index <> OLD.chunk_index OR
    NEW.generation <> OLD.generation OR NEW.model <> OLD.model OR
    NEW.endpoint <> OLD.endpoint OR NEW.split_reason <> OLD.split_reason OR
    NEW.first_ordinal <> OLD.first_ordinal OR NEW.last_ordinal <> OLD.last_ordinal OR
    NEW.input_digest <> OLD.input_digest OR NEW.request_count <> OLD.request_count OR
    NEW.input_bytes <> OLD.input_bytes OR NEW.estimated_prompt_tokens <> OLD.estimated_prompt_tokens THEN
  RAISE EXCEPTION 'prepared-deck Batch chunk identity is immutable';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER deck_preparation_batch_chunks_identity_immutable
 BEFORE UPDATE ON deck_preparation_batch_chunks FOR EACH ROW
 EXECUTE FUNCTION reject_deck_preparation_batch_chunk_identity_mutation();
CREATE TRIGGER deck_preparation_batch_chunk_items_immutable
 BEFORE UPDATE ON deck_preparation_batch_chunk_items FOR EACH ROW
 EXECUTE FUNCTION reject_deck_preparation_manifest_mutation();
