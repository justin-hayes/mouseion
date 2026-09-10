-- Data-only, retry-safe backfill for the one-current-deck change.
--
-- The deployment owner runs this after 000062 and before 000064. Each update
-- is idempotent: book links are filled only when absent, and only rows beyond
-- the newest preparation per Book receive a retirement timestamp. golang-migrate
-- applies this transactionally; row locks and rollback make a failed run safe
-- to retry. After a committed run, recover from backup or use a forward fix,
-- not a destructive down migration.
UPDATE deck_preparations p
SET book_id = s.book_id
FROM source_materials s
WHERE p.book_id IS NULL
  AND s.owner_id = p.owner_id
  AND s.id = p.source_material_id
  AND s.book_id IS NOT NULL;

WITH ranked AS (
  SELECT id,
         row_number() OVER (
           PARTITION BY owner_id, book_id
           ORDER BY COALESCE(completed_at, updated_at, created_at) DESC, id DESC
         ) AS position
  FROM deck_preparations
  WHERE book_id IS NOT NULL
)
UPDATE deck_preparations p
SET retired_at = now()
FROM ranked r
WHERE r.id = p.id
  AND r.position > 1
  AND p.retired_at IS NULL;
