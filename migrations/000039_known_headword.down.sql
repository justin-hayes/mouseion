-- Rolling back drops the stored citation headword of every Known row. Known
-- identity, counts and outcomes are unchanged, but the display form of accepted
-- Greek words is lost and can only be restored by re-importing and re-finishing
-- the affected Readings, so this is not a safe destructive production rollback.
ALTER TABLE public.known_vocabulary
    DROP COLUMN headword;
