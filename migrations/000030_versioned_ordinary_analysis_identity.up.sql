ALTER TABLE analysis_jobs ADD COLUMN analysis_identity text;

DROP INDEX analysis_jobs_legacy_identity;
CREATE UNIQUE INDEX analysis_jobs_legacy_identity
 ON analysis_jobs(owner_id, content_hash, analysis_identity)
 WHERE reviewed_scope_id IS NULL AND analysis_identity IS NOT NULL;
