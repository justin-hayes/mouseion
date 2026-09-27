ALTER TABLE public.deck_preparation_translation_outcomes
    DROP CONSTRAINT deck_preparation_translation_outcomes_omission_state_check,
    DROP CONSTRAINT deck_preparation_translation_outcomes_omission_reason_check,
    DROP COLUMN omission_reason;
