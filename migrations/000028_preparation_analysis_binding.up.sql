-- New deck preparations consume one completed scoped analysis. Nullable
-- provenance preserves readability of preparations created by the legacy
-- book-level workflow.
ALTER TABLE deck_preparations
 ADD COLUMN analysis_run_id uuid,
 ADD CONSTRAINT deck_preparations_analysis_run_fkey
 FOREIGN KEY(owner_id, analysis_run_id, source_material_id)
 REFERENCES analysis_runs(owner_id, id, source_material_id);

ALTER TABLE deck_preparations
 DROP CONSTRAINT deck_preparations_owner_id_source_material_id_content_hash_key;

CREATE UNIQUE INDEX deck_preparations_legacy_identity
 ON deck_preparations(owner_id, source_material_id, content_hash)
 WHERE analysis_run_id IS NULL;

CREATE UNIQUE INDEX deck_preparations_analysis_identity
 ON deck_preparations(owner_id, source_material_id, analysis_run_id)
 WHERE analysis_run_id IS NOT NULL;
