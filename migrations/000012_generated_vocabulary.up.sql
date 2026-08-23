CREATE TABLE generated_vocabulary (
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL,
 canonical_lemma text NOT NULL,
 upos text NOT NULL,
 first_deck_id uuid NOT NULL,
 first_source_material_id uuid,
 first_generated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id, language, canonical_lemma, upos),
 FOREIGN KEY(owner_id, first_deck_id) REFERENCES decks(owner_id, id),
 FOREIGN KEY(owner_id, first_source_material_id) REFERENCES source_materials(owner_id, id)
  ON DELETE SET NULL (first_source_material_id)
);
