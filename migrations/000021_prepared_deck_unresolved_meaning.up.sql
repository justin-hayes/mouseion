ALTER TABLE public.deck_preparation_translation_outcomes
    ADD COLUMN omission_reason text NOT NULL DEFAULT '';

ALTER TABLE public.deck_preparation_translation_outcomes
    ADD CONSTRAINT deck_preparation_translation_outcomes_omission_reason_check
        CHECK (char_length(omission_reason) <= 200 AND omission_reason !~ '[<>]'),
    ADD CONSTRAINT deck_preparation_translation_outcomes_omission_state_check
        CHECK (omission_reason = '' OR (state = 'completed' AND error_class = '' AND error_code = ''));

COMMENT ON COLUMN public.deck_preparation_translation_outcomes.omission_reason IS
    'Validated bounded explanation for a model-confirmed unresolved target meaning; empty for every retryable or failed provider outcome';
