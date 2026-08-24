ALTER TABLE corpora
 ADD COLUMN analyzable_token_count bigint CHECK (analyzable_token_count >= 0),
 ADD COLUMN distinct_lemma_count bigint CHECK (distinct_lemma_count >= 0),
 ADD CONSTRAINT corpora_analysis_statistics_complete CHECK (
  (analyzable_token_count IS NULL) = (distinct_lemma_count IS NULL)
 );

-- Existing artifacts do not retain named-entity filtering information, and
-- selection candidates may already exclude owner vocabulary state. Leaving
-- legacy counts NULL is safer than recording an irreproducible approximation.
