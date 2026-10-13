-- Delete imported Known vocabulary and discard pending imports.
--
-- This migration is data-only: it changes no schema. It ships in the release
-- that retires the `import_known_vocabulary` River worker, so nothing can write
-- an imported row after the deletion.
--
-- Ownership: the release operator applies this with the server's normal
-- migration runner and observes it there. Expected production impact is the loss
-- of imported Known rows (5,020 lemma-wide rows on the 2026-09-16 German import)
-- while every row accepted by Reading completion (1,191 rows) is retained. No
-- table is rewritten; each DELETE takes row locks only.
--
-- Order matters. Pending import jobs are removed first so a queued or running
-- import cannot reinsert rows after the delete. "Queued" covers every non-final
-- River state a worker would still pick up (available, pending, retryable,
-- scheduled); "running" is running. Completed, cancelled and discarded rows are
-- final queue history and stay, and `processing_history` is never touched.
--
-- The `to_regclass` guard keeps a fresh install working: application migrations
-- run before River's schema exists, and a database with no River schema has no
-- import jobs to discard.
--
-- Transaction, failure detection, and retry safety: the migration runner sends
-- this file to PostgreSQL as one multi-statement command, so it runs in a single
-- implicit transaction. Any statement error rolls the whole file back and the
-- server's startup migration reports it; the operator retries the migration.
-- Each DELETE matches only rows that still need removing, so a rerun finds no
-- pending job and no provenance-less row and changes nothing.
--
-- Rollback: the down migration deliberately restores nothing. Imported rows and
-- discarded jobs are recovered only by restoring the pre-migration backup; the
-- deletion is not reversible in place.
DO $$
BEGIN
    IF to_regclass('public.river_job') IS NOT NULL THEN
        DELETE FROM public.river_job
        WHERE kind = 'import_known_vocabulary'
          AND state IN ('available', 'pending', 'running', 'retryable', 'scheduled');
    END IF;
END
$$;

DELETE FROM public.known_vocabulary
WHERE completion_book_id IS NULL;
