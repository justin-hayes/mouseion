-- This restores only the retired nullable shape. The forward backfill's
-- deleted Goal rows and journey memberships are not recoverable by down;
-- restore them from backup if a destructive rollback is ever required.
ALTER TABLE public.primary_goals
    ADD COLUMN reading_finished_at timestamptz;
