DROP TABLE IF EXISTS corpus_tokens;
DROP TABLE IF EXISTS corpus_sentences;

ALTER TABLE corpora
 DROP CONSTRAINT IF EXISTS corpora_owner_id_id_analysis_run_id_key;
