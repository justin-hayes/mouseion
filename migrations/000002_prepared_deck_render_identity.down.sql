ALTER TABLE public.deck_preparations
    DROP CONSTRAINT deck_preparations_presentation_version_check,
    DROP CONSTRAINT deck_preparations_render_input_version_check,
    DROP COLUMN presentation_version,
    DROP COLUMN render_input_version;

ALTER TABLE public.deck_preparation_runs
    DROP CONSTRAINT deck_preparation_runs_presentation_version_check,
    DROP CONSTRAINT deck_preparation_runs_render_input_version_check,
    DROP COLUMN presentation_version,
    DROP COLUMN render_input_version;

ALTER TABLE public.deck_preparation_manifest_items
    DROP CONSTRAINT deck_preparation_manifest_items_sentence_ordinal_check,
    DROP COLUMN sentence_ordinal,
    DROP COLUMN corpus_id;
