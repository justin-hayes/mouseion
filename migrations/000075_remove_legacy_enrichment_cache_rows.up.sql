-- Data-only cleanup for the LLM fallback-gloss cache contract introduced by
-- 000073. Ownership: application startup runs this migration through
-- golang-migrate after the expanded cache identity is installed.
--
-- The predicate makes the cleanup idempotent: a retry finds no rows already
-- removed and does not duplicate any work. PostgreSQL executes this statement
-- in the migration transaction and holds row locks for the affected cache
-- rows; the expected impact is one delete per legacy gloss-only row.
--
-- A migration failure is reported by the runner, rolled back atomically, and is
-- safe to retry after the cause is corrected. The delete is intentionally
-- irreversible: production recovery requires restoring from a database backup
-- or applying an explicitly reviewed forward fix, not a down migration.
DELETE FROM enrichment_cache
WHERE translation = '';
