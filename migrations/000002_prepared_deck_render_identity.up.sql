ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN corpus_id uuid,
    ADD COLUMN sentence_ordinal bigint,
    ADD CONSTRAINT deck_preparation_manifest_items_sentence_ordinal_check
        CHECK (sentence_ordinal IS NULL OR sentence_ordinal >= 0);

ALTER TABLE public.deck_preparation_runs
    ADD COLUMN render_input_version integer NOT NULL DEFAULT 0,
    ADD COLUMN presentation_version integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT deck_preparation_runs_render_input_version_check
        CHECK (render_input_version >= 0),
    ADD CONSTRAINT deck_preparation_runs_presentation_version_check
        CHECK (presentation_version >= 0);

ALTER TABLE public.deck_preparations
    ADD COLUMN render_input_version integer NOT NULL DEFAULT 0,
    ADD COLUMN presentation_version integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT deck_preparations_render_input_version_check
        CHECK (render_input_version >= 0),
    ADD CONSTRAINT deck_preparations_presentation_version_check
        CHECK (presentation_version >= 0);
