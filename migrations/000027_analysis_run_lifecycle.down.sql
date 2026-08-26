DROP INDEX IF EXISTS corpora_run_identity;
ALTER TABLE corpora DROP CONSTRAINT IF EXISTS corpora_analysis_run_fkey;
ALTER TABLE corpora DROP COLUMN IF EXISTS analysis_run_id;

DROP INDEX IF EXISTS analysis_jobs_run_identity;
DROP INDEX IF EXISTS analysis_jobs_legacy_scoped_identity;
ALTER TABLE analysis_jobs DROP CONSTRAINT IF EXISTS analysis_jobs_run_fkey;
ALTER TABLE analysis_jobs DROP COLUMN IF EXISTS analysis_run_id;
CREATE UNIQUE INDEX analysis_jobs_scoped_identity
 ON analysis_jobs(owner_id, reviewed_scope_id) WHERE reviewed_scope_id IS NOT NULL;

DROP TABLE IF EXISTS analysis_run_attempts;
DROP TABLE IF EXISTS analysis_runs;
ALTER TABLE epub_reviewed_scopes DROP CONSTRAINT IF EXISTS epub_reviewed_scopes_run_binding_key;
