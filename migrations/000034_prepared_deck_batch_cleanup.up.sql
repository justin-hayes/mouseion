ALTER TABLE deck_preparation_batch_chunks
 ADD COLUMN input_file_cleanup_state text NOT NULL DEFAULT 'pending',
 ADD COLUMN output_file_cleanup_state text NOT NULL DEFAULT 'pending',
 ADD COLUMN error_file_cleanup_state text NOT NULL DEFAULT 'pending',
 ADD COLUMN input_file_cleanup_attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN output_file_cleanup_attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN error_file_cleanup_attempts integer NOT NULL DEFAULT 0,
 ADD COLUMN cleanup_error_class text NOT NULL DEFAULT '',
 ADD COLUMN cleanup_error_code text NOT NULL DEFAULT '',
 ADD COLUMN cleanup_claim_token uuid,
 ADD COLUMN cleanup_claimed_at timestamptz,
 ADD COLUMN cleanup_lease_expires_at timestamptz,
 ADD COLUMN cleanup_completed_at timestamptz;

ALTER TABLE deck_preparation_batch_chunks
 ADD CONSTRAINT deck_preparation_batch_chunks_cleanup_states_check CHECK (
   input_file_cleanup_state IN ('pending','deleted','failed','not_needed') AND
   output_file_cleanup_state IN ('pending','deleted','failed','not_needed') AND
   error_file_cleanup_state IN ('pending','deleted','failed','not_needed')
 ),
 ADD CONSTRAINT deck_preparation_batch_chunks_cleanup_attempts_check CHECK (
   input_file_cleanup_attempts >= 0 AND output_file_cleanup_attempts >= 0 AND error_file_cleanup_attempts >= 0
 ),
 ADD CONSTRAINT deck_preparation_batch_chunks_cleanup_error_check CHECK (cleanup_error_code ~ '^[a-z0-9_.:-]{0,80}$'),
 ADD CONSTRAINT deck_preparation_batch_chunks_cleanup_claim_check CHECK (
   (cleanup_claim_token IS NULL AND cleanup_claimed_at IS NULL AND cleanup_lease_expires_at IS NULL) OR
   (cleanup_claim_token IS NOT NULL AND cleanup_claimed_at IS NOT NULL AND cleanup_lease_expires_at > cleanup_claimed_at)
 );

CREATE INDEX deck_preparation_batch_chunks_cleanup_idx
 ON deck_preparation_batch_chunks(cleanup_completed_at, updated_at)
 WHERE state IN ('completed','cancelled') AND
       (input_file_cleanup_state IN ('pending','failed') OR
        output_file_cleanup_state IN ('pending','failed') OR
        error_file_cleanup_state IN ('pending','failed'));
