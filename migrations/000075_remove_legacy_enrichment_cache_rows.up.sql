-- Compensating data-only cleanup for the LLM fallback-gloss cache contract
-- introduced by 000073. That migration is immutable shipped history and
-- already performed this cleanup alongside its structural DDL. This standalone
-- predicate is therefore normally a no-op on an ordered upgrade, but remains
-- independently reviewable and safe for a database whose migration marker was
-- advanced while its pre-000073 data shape was restored by an operator.
-- Ownership: application startup runs this migration through golang-migrate
-- after the expanded cache identity is installed.
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
