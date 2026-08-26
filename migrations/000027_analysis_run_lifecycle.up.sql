-- A scoped analysis is a logical, immutable request. River jobs are execution
-- attempts for that request and may be replaced by an explicit retry.
ALTER TABLE epub_reviewed_scopes
 ADD CONSTRAINT epub_reviewed_scopes_run_binding_key
 UNIQUE(scope_id, owner_id, source_material_id, content_revision_id);

CREATE TABLE analysis_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 content_revision_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 analyzer_name text NOT NULL CHECK (btrim(analyzer_name) <> ''),
 analyzer_version text NOT NULL CHECK (btrim(analyzer_version) <> ''),
 config_identity text NOT NULL CHECK (btrim(config_identity) <> ''),
 state text NOT NULL CHECK (state IN ('queued','running','completed','failed','cancelled')),
 attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
 last_error text NOT NULL DEFAULT '',
 corpus_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 completed_at timestamptz,
 UNIQUE(owner_id, id, source_material_id),
 UNIQUE(owner_id, scope_id, analyzer_name, analyzer_version, config_identity),
 FOREIGN KEY(owner_id, source_material_id)
  REFERENCES source_materials(owner_id, id) ON DELETE CASCADE,
 FOREIGN KEY(owner_id, source_material_id, content_revision_id)
  REFERENCES source_content_revisions(owner_id, source_material_id, revision_id),
 FOREIGN KEY(scope_id, owner_id, source_material_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id),
 FOREIGN KEY(scope_id, owner_id, source_material_id, content_revision_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id, content_revision_id),
 FOREIGN KEY(scope_id, snapshot_id)
  REFERENCES epub_reviewed_scopes(scope_id, snapshot_id),
 FOREIGN KEY(owner_id, corpus_id, source_material_id)
  REFERENCES corpora(owner_id, id, source_material_id)
);

CREATE TABLE analysis_run_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 run_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 attempt_number integer NOT NULL CHECK (attempt_number > 0),
 river_job_id bigint NOT NULL UNIQUE,
 state text NOT NULL CHECK (state IN ('queued','running','completed','failed','cancelled')),
 error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finalized_at timestamptz,
 UNIQUE(run_id, attempt_number),
 FOREIGN KEY(owner_id, run_id, source_material_id)
  REFERENCES analysis_runs(owner_id, id, source_material_id) ON DELETE CASCADE
);

ALTER TABLE analysis_jobs ADD COLUMN analysis_run_id uuid;
ALTER TABLE analysis_jobs
 ADD CONSTRAINT analysis_jobs_run_fkey
 FOREIGN KEY(owner_id, analysis_run_id, source_material_id)
 REFERENCES analysis_runs(owner_id, id, source_material_id);
DROP INDEX analysis_jobs_scoped_identity;
CREATE UNIQUE INDEX analysis_jobs_legacy_scoped_identity
 ON analysis_jobs(owner_id, reviewed_scope_id)
 WHERE reviewed_scope_id IS NOT NULL AND analysis_run_id IS NULL;
CREATE UNIQUE INDEX analysis_jobs_run_identity
 ON analysis_jobs(owner_id, analysis_run_id)
 WHERE analysis_run_id IS NOT NULL;

ALTER TABLE corpora ADD COLUMN analysis_run_id uuid;
ALTER TABLE corpora
 ADD CONSTRAINT corpora_analysis_run_fkey
 FOREIGN KEY(owner_id, analysis_run_id, source_material_id)
 REFERENCES analysis_runs(owner_id, id, source_material_id);
-- Keep the pre-lifecycle scope identity for legacy scoped corpora. New runs
-- additionally have their own immutable run identity below.
CREATE UNIQUE INDEX corpora_run_identity
 ON corpora(owner_id, analysis_run_id)
 WHERE analysis_run_id IS NOT NULL;
