-- Study corrections (ADR 0134 §6, spec #2691, ticket #2701): when a Lemma
-- correction, an Occurrence exclusion or an Analysis update leaves a word's
-- introducing occurrence resolving to another identity, the word's next review
-- is a re-introduction in new Study material. corrected_at is when the word
-- was re-introduced, so its card can say it was corrected; it is null for a
-- word that never was. Additive and nullable, with one consumer: the Study
-- page. The FSRS state, due time and review log are not touched.
ALTER TABLE public.study_progress ADD COLUMN corrected_at timestamptz;

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
