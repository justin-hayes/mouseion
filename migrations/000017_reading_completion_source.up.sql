ALTER TABLE public.reading_history
    ADD COLUMN completion_source text NOT NULL DEFAULT 'primary_goal',
    ADD CONSTRAINT reading_history_completion_source_check
        CHECK (completion_source IN ('primary_goal', 'previously_read_import'));

CREATE UNIQUE INDEX reading_history_previously_read_import_identity_idx
    ON public.reading_history(owner_id, book_id)
    WHERE completion_source = 'previously_read_import';
