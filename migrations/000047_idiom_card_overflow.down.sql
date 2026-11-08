ALTER TABLE public.deck_preparation_manifests
    DROP CONSTRAINT deck_preparation_manifests_idiom_overflow_check;

ALTER TABLE public.deck_preparation_manifests
    DROP COLUMN idiom_overflow;
