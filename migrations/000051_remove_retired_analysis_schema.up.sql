-- ADR 0047 / issue #616: the catalogue and immutable EPUB snapshots are now
-- the only supported library and analysis inputs.
--
-- This migration is intentionally destructive. The deployment owner must take
-- and verify a database backup before applying it. Deleted manual books and
-- plain-text source material are recoverable only by restoring that backup or
-- re-syncing the learner's catalogue; the down migration cannot restore data.
-- The statements are safe to retry after a failed deployment.

-- Manual books are dead library data. ON DELETE CASCADE removes memberships,
-- aliases, journey membership, goals, and current-analysis projections.
-- Clear the composite source-material book reference first: PostgreSQL's
-- ON DELETE SET NULL would otherwise null the owner_id component as well.
UPDATE source_materials
SET book_id = NULL
WHERE book_id IN (SELECT id FROM books WHERE metadata_provenance = 'manual_entry');
DELETE FROM books WHERE metadata_provenance = 'manual_entry';

-- Plain-text sources are no longer analyzable and are not catalogue recovery
-- material. Their dependent analysis/corpus data follows the source FKs.
DELETE FROM learning_campaigns
 WHERE source_material_id IN (
   SELECT id FROM source_materials WHERE media_type <> 'application/epub+zip'
 );
DELETE FROM book_current_analyses
 WHERE source_material_id IN (
   SELECT id FROM source_materials WHERE media_type <> 'application/epub+zip'
 );
DELETE FROM source_materials WHERE media_type <> 'application/epub+zip';

-- Remove the reviewed-scope evidence before its referenced scope tables.
DROP TABLE IF EXISTS corpus_selected_units CASCADE;
DROP TABLE IF EXISTS epub_reviewed_scope_units CASCADE;
DROP TABLE IF EXISTS epub_reviewed_scopes CASCADE;

-- CASCADE removes the historical scope foreign keys, indexes, and constraints.
ALTER TABLE IF EXISTS analysis_runs
  DROP COLUMN IF EXISTS scope_id CASCADE;
ALTER TABLE IF EXISTS analysis_jobs
  DROP COLUMN IF EXISTS reviewed_scope_id CASCADE;
ALTER TABLE IF EXISTS corpora
  DROP COLUMN IF EXISTS reviewed_scope_id CASCADE;
