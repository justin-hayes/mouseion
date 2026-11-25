-- Discard queued per-Book `book_cover` River jobs (ADR 0130, ADR 0038).
--
-- Data-only: no schema changes. The per-Book retrieval worker is retired in the
-- same deploy as 000064-000066, so a job still queued would fail as an
-- unregistered kind until River discarded it. Covers still waiting are
-- retrieved by the next Catalog sync instead (entry retrieval jobs).
--
-- Ownership: the release operator, through the normal migration Up. Only
-- non-final states are removed ("queued": available, pending, retryable,
-- scheduled; and running); completed, cancelled and discarded rows stay as
-- queue history. The to_regclass guard keeps a fresh install working, since
-- application migrations run before River's schema exists. Each statement runs
-- in the file's single implicit transaction, takes row locks only, and matches
-- only rows that still need removing, so a rerun changes nothing.
--
-- Rollback: nothing is restored; the discarded jobs cannot run without their
-- worker. Recovery is the next Catalog sync.
DO $$
BEGIN
    IF to_regclass('public.river_job') IS NOT NULL THEN
        DELETE FROM public.river_job
        WHERE kind = 'book_cover'
          AND state IN ('available', 'pending', 'running', 'retryable', 'scheduled');
    END IF;
END
$$;
