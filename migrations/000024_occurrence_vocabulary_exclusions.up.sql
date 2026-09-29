ALTER TABLE public.occurrence_lemma_corrections
    ADD COLUMN excluded boolean NOT NULL DEFAULT false;

ALTER TABLE public.occurrence_lemma_corrections
    ALTER COLUMN canonical_lemma DROP NOT NULL,
    ADD CONSTRAINT occurrence_lemma_corrections_decision_check
        CHECK ((excluded AND canonical_lemma IS NULL) OR (NOT excluded AND canonical_lemma IS NOT NULL));

ALTER TABLE public.occurrence_lemma_corrections
    ALTER COLUMN normalization_profile DROP NOT NULL,
    ALTER COLUMN normalization_version DROP NOT NULL;

ALTER TABLE public.occurrence_lemma_corrections
    ADD CONSTRAINT occurrence_lemma_corrections_exclusion_metadata_check
        CHECK (NOT excluded OR (normalization_profile IS NULL AND normalization_version IS NULL));
