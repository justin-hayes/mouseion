DROP INDEX example_sentences_identity_chosen_idx;
DROP INDEX example_sentences_identity_rank_idx;
ALTER TABLE example_sentences DROP CONSTRAINT example_sentences_selection_metadata_check;
ALTER TABLE example_sentences
 DROP COLUMN is_chosen,
 DROP COLUMN selection_reasons,
 DROP COLUMN selection_score,
 DROP COLUMN selection_rank,
 DROP COLUMN upos,
 DROP COLUMN canonical_lemma,
 DROP COLUMN language;
