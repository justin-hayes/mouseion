-- Report how many candidate idioms the per-deck bound left out (ticket #2365;
-- doc/features/idiom-cards.md). The overflow belongs to the frozen manifest,
-- not to an omission, so it is additive metadata and no existing row changes
-- meaning. Existing rows report zero overflow.

ALTER TABLE public.deck_preparation_manifests
    ADD COLUMN idiom_overflow integer NOT NULL DEFAULT 0;

ALTER TABLE public.deck_preparation_manifests
    ADD CONSTRAINT deck_preparation_manifests_idiom_overflow_check
        CHECK (idiom_overflow >= 0);

COMMENT ON COLUMN public.deck_preparation_manifests.idiom_overflow IS
    'Candidate idioms left out by the per-deck idiom-card bound; not an omission';
