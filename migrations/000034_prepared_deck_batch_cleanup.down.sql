DROP INDEX IF EXISTS deck_preparation_batch_chunks_cleanup_idx;
ALTER TABLE deck_preparation_batch_chunks
 DROP CONSTRAINT IF EXISTS deck_preparation_batch_chunks_cleanup_claim_check,
 DROP CONSTRAINT IF EXISTS deck_preparation_batch_chunks_cleanup_error_check,
 DROP CONSTRAINT IF EXISTS deck_preparation_batch_chunks_cleanup_attempts_check,
 DROP CONSTRAINT IF EXISTS deck_preparation_batch_chunks_cleanup_states_check,
 DROP COLUMN IF EXISTS cleanup_completed_at,
 DROP COLUMN IF EXISTS cleanup_lease_expires_at,
 DROP COLUMN IF EXISTS cleanup_claimed_at,
 DROP COLUMN IF EXISTS cleanup_claim_token,
 DROP COLUMN IF EXISTS cleanup_error_code,
 DROP COLUMN IF EXISTS cleanup_error_class,
 DROP COLUMN IF EXISTS error_file_cleanup_attempts,
 DROP COLUMN IF EXISTS output_file_cleanup_attempts,
 DROP COLUMN IF EXISTS input_file_cleanup_attempts,
 DROP COLUMN IF EXISTS error_file_cleanup_state,
 DROP COLUMN IF EXISTS output_file_cleanup_state,
 DROP COLUMN IF EXISTS input_file_cleanup_state;
