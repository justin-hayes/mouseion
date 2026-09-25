DROP INDEX public.reading_history_previously_read_import_identity_idx;

ALTER TABLE public.reading_history
    DROP CONSTRAINT reading_history_completion_source_check,
    DROP COLUMN completion_source;
