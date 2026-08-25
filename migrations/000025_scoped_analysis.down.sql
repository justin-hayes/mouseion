DROP TABLE corpus_selected_units;

ALTER TABLE epub_reviewed_scopes
 DROP CONSTRAINT epub_reviewed_scopes_scope_snapshot_key;

DROP INDEX corpora_scoped_identity;
DROP INDEX corpora_legacy_source_identity;
ALTER TABLE corpora DROP CONSTRAINT corpora_reviewed_scope_fkey;
ALTER TABLE corpora DROP CONSTRAINT corpora_owner_id_id_source_material_id_key;
ALTER TABLE corpora DROP COLUMN reviewed_scope_id;
ALTER TABLE corpora ADD UNIQUE(owner_id, source_material_id);

DROP INDEX analysis_jobs_scoped_identity;
DROP INDEX analysis_jobs_legacy_identity;
ALTER TABLE analysis_jobs DROP CONSTRAINT analysis_jobs_reviewed_scope_fkey;
ALTER TABLE analysis_jobs DROP COLUMN reviewed_scope_id;
ALTER TABLE analysis_jobs ADD UNIQUE(owner_id, content_hash);
