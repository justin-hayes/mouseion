DROP INDEX analysis_jobs_legacy_identity;
ALTER TABLE analysis_jobs DROP COLUMN analysis_identity;

CREATE UNIQUE INDEX analysis_jobs_legacy_identity
 ON analysis_jobs(owner_id, content_hash)
 WHERE reviewed_scope_id IS NULL;
