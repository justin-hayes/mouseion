ALTER TABLE public.occurrence_lemma_corrections
    DROP CONSTRAINT occurrence_lemma_corrections_exclusion_metadata_check,
    DROP CONSTRAINT occurrence_lemma_corrections_decision_check;

DELETE FROM public.occurrence_lemma_corrections WHERE excluded;

ALTER TABLE public.occurrence_lemma_corrections
    ALTER COLUMN canonical_lemma SET NOT NULL,
    ALTER COLUMN normalization_profile SET NOT NULL,
    ALTER COLUMN normalization_version SET NOT NULL,
    DROP COLUMN excluded;
