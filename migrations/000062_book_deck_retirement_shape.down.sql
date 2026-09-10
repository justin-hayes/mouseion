DROP INDEX IF EXISTS deck_preparations_legacy_identity;

ALTER TABLE deck_preparations
  DROP CONSTRAINT IF EXISTS deck_preparations_book_fkey,
  DROP COLUMN IF EXISTS retired_at,
  DROP COLUMN IF EXISTS book_id;

CREATE UNIQUE INDEX deck_preparations_legacy_identity
  ON deck_preparations(owner_id, source_material_id, content_hash)
  WHERE analysis_run_id IS NULL;
