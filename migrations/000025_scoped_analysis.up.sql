ALTER TABLE analysis_jobs
 ADD COLUMN reviewed_scope_id uuid,
 ADD CONSTRAINT analysis_jobs_reviewed_scope_fkey
  FOREIGN KEY(reviewed_scope_id, owner_id, source_material_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id);

ALTER TABLE analysis_jobs DROP CONSTRAINT analysis_jobs_owner_id_content_hash_key;
CREATE UNIQUE INDEX analysis_jobs_legacy_identity
 ON analysis_jobs(owner_id, content_hash) WHERE reviewed_scope_id IS NULL;
CREATE UNIQUE INDEX analysis_jobs_scoped_identity
 ON analysis_jobs(owner_id, reviewed_scope_id) WHERE reviewed_scope_id IS NOT NULL;

ALTER TABLE corpora
 ADD COLUMN reviewed_scope_id uuid,
 ADD CONSTRAINT corpora_owner_id_id_source_material_id_key UNIQUE(owner_id, id, source_material_id),
 ADD CONSTRAINT corpora_reviewed_scope_fkey
  FOREIGN KEY(reviewed_scope_id, owner_id, source_material_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id);

ALTER TABLE corpora DROP CONSTRAINT corpora_owner_id_source_material_id_key;
CREATE UNIQUE INDEX corpora_legacy_source_identity
 ON corpora(owner_id, source_material_id) WHERE reviewed_scope_id IS NULL;
CREATE UNIQUE INDEX corpora_scoped_identity
 ON corpora(owner_id, reviewed_scope_id) WHERE reviewed_scope_id IS NOT NULL;

ALTER TABLE epub_reviewed_scopes
 ADD CONSTRAINT epub_reviewed_scopes_scope_snapshot_key UNIQUE(scope_id, snapshot_id);

CREATE TABLE corpus_selected_units (
 corpus_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 source_material_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 unit_id text NOT NULL,
 unit_order bigint NOT NULL CHECK (unit_order >= 0),
 source_href text NOT NULL,
 resolved_href text NOT NULL,
 title text NOT NULL,
 start_offset bigint NOT NULL CHECK (start_offset >= 0),
 end_offset bigint NOT NULL CHECK (end_offset >= start_offset),
 PRIMARY KEY(corpus_id, unit_id),
 UNIQUE(corpus_id, unit_order),
 FOREIGN KEY(owner_id, corpus_id, source_material_id)
  REFERENCES corpora(owner_id, id, source_material_id) ON DELETE CASCADE,
 FOREIGN KEY(scope_id, owner_id, source_material_id)
  REFERENCES epub_reviewed_scopes(scope_id, owner_id, source_material_id),
 FOREIGN KEY(scope_id, snapshot_id)
  REFERENCES epub_reviewed_scopes(scope_id, snapshot_id),
 FOREIGN KEY(scope_id, unit_id)
  REFERENCES epub_reviewed_scope_units(scope_id, unit_id),
 FOREIGN KEY(scope_id, unit_order)
  REFERENCES epub_reviewed_scope_units(scope_id, unit_order)
);
