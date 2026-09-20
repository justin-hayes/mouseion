ALTER TABLE public.reading_history
    DROP CONSTRAINT reading_history_completion_identity_key,
    DROP CONSTRAINT reading_history_pkey,
    DROP COLUMN completion_id,
    ADD CONSTRAINT reading_history_pkey PRIMARY KEY (owner_id, language, book_id);
