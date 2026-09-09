ALTER TABLE deck_preparations
 ADD COLUMN studying_at timestamptz,
 ADD COLUMN reviewed_at timestamptz,
 ADD COLUMN graduated_at timestamptz,
 ADD COLUMN released_at timestamptz;

CREATE UNIQUE INDEX deck_preparations_one_studying_per_owner
 ON deck_preparations(owner_id) WHERE studying_at IS NOT NULL;

CREATE TABLE deck_preparation_vocabulary (
 owner_id uuid NOT NULL,
 deck_preparation_id uuid NOT NULL,
 language text NOT NULL,
 canonical_lemma text NOT NULL,
 upos text NOT NULL,
 generated_at timestamptz NOT NULL,
 graduated_at timestamptz,
 PRIMARY KEY(owner_id, deck_preparation_id, language, canonical_lemma, upos),
 FOREIGN KEY(owner_id, deck_preparation_id) REFERENCES deck_preparations(owner_id, id) ON DELETE CASCADE,
 FOREIGN KEY(owner_id, language, canonical_lemma, upos)
  REFERENCES generated_vocabulary(owner_id, language, canonical_lemma, upos)
);

CREATE INDEX deck_preparation_vocabulary_owner_identity_idx
 ON deck_preparation_vocabulary(owner_id, language, canonical_lemma, upos);
