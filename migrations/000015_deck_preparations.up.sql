CREATE TABLE deck_preparations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_material_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','preparing','ready','failed','cancelled')),
 artifact bytea,
 filename text NOT NULL,
 deck_name text NOT NULL,
 content_hash text NOT NULL,
 total_cards integer NOT NULL DEFAULT 0 CHECK(total_cards >= 0),
 cards_with_english integer NOT NULL DEFAULT 0 CHECK(cards_with_english >= 0),
 cards_with_contextual_sentence_translations integer NOT NULL DEFAULT 0 CHECK(cards_with_contextual_sentence_translations >= 0),
 quality_omissions integer NOT NULL DEFAULT 0 CHECK(quality_omissions >= 0),
 error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 completed_at timestamptz,
 UNIQUE(owner_id, source_material_id, content_hash),
 UNIQUE(owner_id, id),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id) ON DELETE CASCADE,
 CHECK((state = 'ready' AND artifact IS NOT NULL AND octet_length(artifact) > 0 AND completed_at IS NOT NULL) OR
       (state <> 'ready' AND artifact IS NULL)),
 CHECK(cards_with_english <= total_cards),
 CHECK(cards_with_contextual_sentence_translations <= total_cards)
);

CREATE INDEX deck_preparations_owner_updated_idx ON deck_preparations(owner_id, updated_at DESC);
