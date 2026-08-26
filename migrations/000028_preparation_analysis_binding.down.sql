DROP INDEX IF EXISTS deck_preparations_analysis_identity;
DROP INDEX IF EXISTS deck_preparations_legacy_identity;

ALTER TABLE deck_preparations
 DROP CONSTRAINT IF EXISTS deck_preparations_analysis_run_fkey,
 DROP COLUMN IF EXISTS analysis_run_id;

ALTER TABLE deck_preparations
 ADD CONSTRAINT deck_preparations_owner_id_source_material_id_content_hash_key
 UNIQUE(owner_id, source_material_id, content_hash);
