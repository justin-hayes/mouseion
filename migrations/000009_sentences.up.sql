ALTER TABLE example_sentences
 ADD COLUMN language text,
 ADD COLUMN canonical_lemma text,
 ADD COLUMN upos text,
 ADD COLUMN selection_rank integer,
 ADD COLUMN selection_score integer,
 ADD COLUMN selection_reasons jsonb,
 ADD COLUMN is_chosen boolean NOT NULL DEFAULT false;

ALTER TABLE example_sentences
 ADD CONSTRAINT example_sentences_selection_metadata_check CHECK (
  (language IS NULL AND canonical_lemma IS NULL AND upos IS NULL AND selection_rank IS NULL AND selection_score IS NULL AND selection_reasons IS NULL)
  OR
  (language <> '' AND canonical_lemma <> '' AND upos <> '' AND selection_rank >= 1 AND selection_reasons IS NOT NULL)
 );

CREATE UNIQUE INDEX example_sentences_identity_rank_idx
 ON example_sentences(owner_id, corpus_id, language, canonical_lemma, upos, selection_rank)
 WHERE language IS NOT NULL;
CREATE UNIQUE INDEX example_sentences_identity_chosen_idx
 ON example_sentences(owner_id, corpus_id, language, canonical_lemma, upos)
 WHERE is_chosen;
