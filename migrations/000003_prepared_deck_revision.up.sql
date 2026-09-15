ALTER TABLE public.deck_preparations
    ADD COLUMN deck_revision integer NOT NULL DEFAULT 1,
    ADD CONSTRAINT deck_preparations_deck_revision_check
        CHECK (deck_revision > 0);
