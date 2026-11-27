-- Recreating the index fails if a Book was finished twice without a snapshot
-- after the up migration; that history is append-only and is not rewritten, so
-- the down migration is not assumed safe for destructive production rollback.
CREATE UNIQUE INDEX IF NOT EXISTS reading_history_null_snapshot_identity_idx
    ON public.reading_history(owner_id, language, book_id)
    WHERE goal_snapshot_id IS NULL;
