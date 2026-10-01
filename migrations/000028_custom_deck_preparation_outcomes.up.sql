ALTER TABLE public.custom_vocabulary_deck_preparations
    ADD COLUMN omissions jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE public.custom_vocabulary_deck_preparations
    DROP CONSTRAINT custom_vocabulary_deck_preparations_state_check,
    ADD CONSTRAINT custom_vocabulary_deck_preparations_state_check
        CHECK (state IN ('queued', 'preparing', 'ready', 'complete_with_omissions', 'failed', 'cancelled')),
    DROP CONSTRAINT custom_vocabulary_deck_preparations_ready_check,
    ADD CONSTRAINT custom_vocabulary_deck_preparations_ready_check
        CHECK ((state IN ('ready', 'complete_with_omissions')) = (artifact IS NOT NULL AND completed_at IS NOT NULL AND total_cards > 0)),
    ADD CONSTRAINT custom_vocabulary_deck_preparations_omissions_check
        CHECK (jsonb_typeof(omissions) = 'array');

CREATE INDEX custom_vocabulary_deck_preparations_latest_ready
    ON public.custom_vocabulary_deck_preparations(owner_id, custom_deck_id, created_at DESC, id DESC)
    WHERE state IN ('ready', 'complete_with_omissions');
