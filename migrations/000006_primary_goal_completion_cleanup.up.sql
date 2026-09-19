-- Finished reading is now represented by reading_history, never by mutable
-- Primary Goal state. This runs after the separate legacy backfill.
ALTER TABLE public.primary_goals
    DROP COLUMN reading_finished_at;
