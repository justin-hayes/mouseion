-- Persist the complete normalized corpus for future concordance queries. The
-- run identity is part of every row's key so completed analyses remain
-- immutable history even when a Book receives a newer current analysis.
ALTER TABLE corpora
 ADD CONSTRAINT corpora_owner_id_id_analysis_run_id_key
 UNIQUE(owner_id, id, analysis_run_id);

CREATE TABLE corpus_sentences (
 owner_id uuid NOT NULL,
 analysis_run_id uuid NOT NULL,
 corpus_id uuid NOT NULL,
 unit_id text NOT NULL CHECK (unit_id <> ''),
 sentence_ordinal bigint NOT NULL CHECK (sentence_ordinal >= 0),
 sentence_text text NOT NULL,
 start_offset bigint NOT NULL CHECK (start_offset >= 0),
 end_offset bigint NOT NULL CHECK (end_offset >= start_offset),
 PRIMARY KEY(analysis_run_id, sentence_ordinal),
 UNIQUE(owner_id, corpus_id, analysis_run_id, sentence_ordinal),
 FOREIGN KEY(owner_id, corpus_id, analysis_run_id)
  REFERENCES corpora(owner_id, id, analysis_run_id) ON DELETE CASCADE
);

CREATE TABLE corpus_tokens (
 owner_id uuid NOT NULL,
 language text NOT NULL CHECK (language <> ''),
 analysis_run_id uuid NOT NULL,
 corpus_id uuid NOT NULL,
 sentence_ordinal bigint NOT NULL CHECK (sentence_ordinal >= 0),
 token_ordinal bigint NOT NULL CHECK (token_ordinal >= 0),
 surface text NOT NULL,
 raw_lemma text NOT NULL,
 canonical_lemma text NOT NULL,
 upos text NOT NULL,
 morphology jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(morphology) = 'object'),
 named_entity text,
 start_offset bigint NOT NULL CHECK (start_offset >= 0),
 end_offset bigint NOT NULL CHECK (end_offset >= start_offset),
 PRIMARY KEY(analysis_run_id, sentence_ordinal, token_ordinal),
 FOREIGN KEY(owner_id, corpus_id, analysis_run_id, sentence_ordinal)
  REFERENCES corpus_sentences(owner_id, corpus_id, analysis_run_id, sentence_ordinal)
  ON DELETE CASCADE
);

CREATE INDEX corpus_tokens_owner_language_canonical_lemma_upos_idx
 ON corpus_tokens(owner_id, language, canonical_lemma, upos);

CREATE INDEX corpus_tokens_owner_language_surface_idx
 ON corpus_tokens(owner_id, language, surface);
