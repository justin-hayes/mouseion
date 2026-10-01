DROP INDEX public.custom_vocabulary_deck_preparations_latest_ready;

UPDATE public.custom_vocabulary_deck_preparations
SET state = 'ready'
WHERE state = 'complete_with_omissions';

ALTER TABLE public.custom_vocabulary_deck_preparations
    DROP CONSTRAINT custom_vocabulary_deck_preparations_omissions_check,
    DROP COLUMN omissions,
    DROP CONSTRAINT custom_vocabulary_deck_preparations_ready_check,
    ADD CONSTRAINT custom_vocabulary_deck_preparations_ready_check
        CHECK ((state = 'ready') = (artifact IS NOT NULL AND completed_at IS NOT NULL AND total_cards > 0)),
    DROP CONSTRAINT custom_vocabulary_deck_preparations_state_check,
    ADD CONSTRAINT custom_vocabulary_deck_preparations_state_check
        CHECK (state IN ('queued', 'preparing', 'ready', 'failed', 'cancelled'));
