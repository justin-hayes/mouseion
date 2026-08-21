CREATE TABLE selection_candidates (
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 corpus_id text NOT NULL,
 language text NOT NULL,
 canonical_lemma text NOT NULL,
 upos text NOT NULL,
 occurrence_count integer NOT NULL CHECK (occurrence_count > 0),
 observed_forms jsonb NOT NULL,
 eligible_sentence_refs jsonb NOT NULL,
 provenance jsonb NOT NULL,
 selected_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id, corpus_id, language, canonical_lemma, upos)
);
CREATE INDEX selection_candidates_owner_identity_idx
 ON selection_candidates(owner_id, language, canonical_lemma, upos);
