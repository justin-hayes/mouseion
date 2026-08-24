ALTER TABLE corpora
 DROP CONSTRAINT corpora_structural_statistics_complete,
 DROP COLUMN long_sentence_count,
 DROP COLUMN p90_sentence_token_count,
 DROP COLUMN median_sentence_token_count,
 DROP COLUMN empty_sentence_count,
 DROP COLUMN normalized_token_count,
 DROP COLUMN sentence_count;
