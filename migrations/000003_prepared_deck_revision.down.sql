ALTER TABLE public.deck_preparations
    DROP CONSTRAINT deck_preparations_deck_revision_check,
    DROP COLUMN deck_revision;
