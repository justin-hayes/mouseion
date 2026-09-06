-- New analysis runs are keyed by immutable source content revisions, not by
-- learner-reviewed scopes. Existing scoped runs and scope rows remain intact.
ALTER TABLE analysis_runs
  ALTER COLUMN scope_id DROP NOT NULL;

ALTER TABLE analysis_runs
  DROP CONSTRAINT IF EXISTS analysis_runs_owner_id_scope_id_analyzer_name_analyzer_version_config_identity_key;
CREATE UNIQUE INDEX analysis_runs_legacy_scope_identity
  ON analysis_runs(owner_id, scope_id, analyzer_name, analyzer_version, config_identity)
  WHERE scope_id IS NOT NULL;
CREATE UNIQUE INDEX analysis_runs_snapshot_identity
  ON analysis_runs(owner_id, content_revision_id, analyzer_name, analyzer_version, config_identity)
  WHERE scope_id IS NULL;
