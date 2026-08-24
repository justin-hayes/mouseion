ALTER TABLE corpora
 ADD COLUMN sentence_count bigint CHECK (sentence_count >= 0),
 ADD COLUMN normalized_token_count bigint CHECK (normalized_token_count >= 0),
 ADD COLUMN empty_sentence_count bigint CHECK (empty_sentence_count >= 0),
 ADD COLUMN median_sentence_token_count double precision CHECK (median_sentence_token_count >= 0),
 ADD COLUMN p90_sentence_token_count bigint CHECK (p90_sentence_token_count >= 0),
 ADD COLUMN long_sentence_count bigint CHECK (long_sentence_count >= 0),
 ADD CONSTRAINT corpora_structural_statistics_complete CHECK (
  (sentence_count IS NULL) = (normalized_token_count IS NULL)
  AND (sentence_count IS NULL) = (empty_sentence_count IS NULL)
  AND (sentence_count IS NULL) = (median_sentence_token_count IS NULL)
  AND (sentence_count IS NULL) = (p90_sentence_token_count IS NULL)
  AND (sentence_count IS NULL) = (long_sentence_count IS NULL)
  AND (sentence_count IS NULL OR empty_sentence_count <= sentence_count)
  AND (sentence_count IS NULL OR long_sentence_count <= sentence_count)
 );

-- The normalized sentence stream is not retained, so legacy corpora cannot be
-- backfilled reproducibly and keep these fields NULL until reanalysis.
