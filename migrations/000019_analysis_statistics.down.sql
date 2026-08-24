ALTER TABLE corpora
 DROP CONSTRAINT corpora_analysis_statistics_complete,
 DROP COLUMN distinct_lemma_count,
 DROP COLUMN analyzable_token_count;
