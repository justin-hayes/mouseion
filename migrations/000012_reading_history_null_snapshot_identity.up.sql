-- NULL snapshot completions need a real identity so concurrent retries cannot
-- create duplicate history rows after the Goal has been removed.
CREATE UNIQUE INDEX reading_history_null_snapshot_identity_idx
    ON public.reading_history(owner_id, language, book_id)
    WHERE goal_snapshot_id IS NULL;
