-- Completion history is keyed by the immutable Goal snapshot, not by the
-- Book. This preserves legacy rows while allowing a Book to be completed
-- again after it is returned to the Reading Journey.
ALTER TABLE public.reading_history
    ADD COLUMN completion_id uuid DEFAULT gen_random_uuid() NOT NULL;

ALTER TABLE public.reading_history
    DROP CONSTRAINT reading_history_pkey,
    ADD CONSTRAINT reading_history_pkey PRIMARY KEY (completion_id),
    ADD CONSTRAINT reading_history_completion_identity_key
        UNIQUE (owner_id, language, goal_snapshot_id);

-- Legacy Goals without trustworthy current analysis have no snapshot. Give
-- their completion retries a durable identity as well.
CREATE UNIQUE INDEX reading_history_null_snapshot_identity_idx
    ON public.reading_history(owner_id, language, book_id)
    WHERE goal_snapshot_id IS NULL;
