-- Structural half of the one-current-deck change. The data backfill and
-- current-row index are separate migrations so each operation can be observed
-- and recovered independently.
ALTER TABLE deck_preparations
  ADD COLUMN book_id uuid,
  ADD COLUMN retired_at timestamptz;

ALTER TABLE deck_preparations
  ADD CONSTRAINT deck_preparations_book_fkey
  FOREIGN KEY (owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE;

DROP INDEX IF EXISTS deck_preparations_legacy_identity;
CREATE UNIQUE INDEX deck_preparations_legacy_identity
  ON deck_preparations(owner_id, source_material_id, content_hash)
  WHERE book_id IS NULL AND analysis_run_id IS NULL;
