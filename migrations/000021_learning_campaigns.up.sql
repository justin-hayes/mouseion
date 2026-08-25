CREATE TABLE learning_campaigns (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_material_id uuid NOT NULL,
 deck_preparation_id uuid NOT NULL,
 book_status text NOT NULL DEFAULT 'queued' CHECK(book_status IN ('queued','reading','finished','abandoned')),
 deck_status text NOT NULL DEFAULT 'queued' CHECK(deck_status IN ('queued','studying','reviewed','abandoned')),
 status text GENERATED ALWAYS AS (
  CASE
   WHEN book_status = 'abandoned' OR deck_status = 'abandoned' THEN 'abandoned'
   WHEN book_status = 'finished' AND deck_status = 'reviewed' THEN 'complete'
   WHEN book_status = 'queued' AND deck_status = 'queued' THEN 'queued'
   ELSE 'active'
  END
 ) STORED,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 activated_at timestamptz,
 book_finished_at timestamptz,
 deck_reviewed_at timestamptz,
 completed_at timestamptz,
 abandoned_at timestamptz,
 vocabulary_graduated_at timestamptz,
 UNIQUE(owner_id, id),
 UNIQUE(owner_id, deck_preparation_id),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id),
 FOREIGN KEY(owner_id, deck_preparation_id) REFERENCES deck_preparations(owner_id, id),
 CHECK((book_status = 'finished') = (book_finished_at IS NOT NULL)),
 CHECK((deck_status = 'reviewed') = (deck_reviewed_at IS NOT NULL)),
 CHECK(((book_status = 'finished' AND deck_status = 'reviewed')) = (completed_at IS NOT NULL)),
 CHECK(((book_status = 'abandoned' OR deck_status = 'abandoned')) = (abandoned_at IS NOT NULL)),
 CHECK(vocabulary_graduated_at IS NULL OR (book_status = 'finished' AND deck_status = 'reviewed'))
);

CREATE UNIQUE INDEX learning_campaigns_one_active_per_owner
 ON learning_campaigns(owner_id) WHERE status = 'active';
CREATE INDEX learning_campaigns_owner_created_idx
 ON learning_campaigns(owner_id, created_at, id);

-- This snapshot is the explicit relationship between immutable generated
-- history and a campaign. Unrelated legacy rows remain provenance only.
CREATE TABLE learning_campaign_vocabulary (
 owner_id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 language text NOT NULL,
 canonical_lemma text NOT NULL,
 upos text NOT NULL,
 generated_at timestamptz NOT NULL,
 graduated_at timestamptz,
 PRIMARY KEY(owner_id, campaign_id, language, canonical_lemma, upos),
 FOREIGN KEY(owner_id, campaign_id) REFERENCES learning_campaigns(owner_id, id) ON DELETE CASCADE,
 FOREIGN KEY(owner_id, language, canonical_lemma, upos)
  REFERENCES generated_vocabulary(owner_id, language, canonical_lemma, upos)
);
