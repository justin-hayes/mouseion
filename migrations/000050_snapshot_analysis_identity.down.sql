DROP INDEX IF EXISTS analysis_runs_snapshot_identity;
DROP INDEX IF EXISTS analysis_runs_legacy_scope_identity;
ALTER TABLE analysis_runs
  ADD CONSTRAINT analysis_runs_owner_id_scope_id_analyzer_name_analyzer_version_config_identity_key
  UNIQUE(owner_id, scope_id, analyzer_name, analyzer_version, config_identity);
ALTER TABLE analysis_runs
  ALTER COLUMN scope_id SET NOT NULL;
ALTER TABLE analysis_runs
  ALTER COLUMN scope_id SET NOT NULL;
