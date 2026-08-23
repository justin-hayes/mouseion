-- Backfill explicit generated history from cards created before that history
-- was recorded. Cards do not retain source-material provenance, so it must
-- remain NULL rather than being inferred.
INSERT INTO generated_vocabulary (
 owner_id, language, canonical_lemma, upos, first_deck_id,
 first_source_material_id, first_generated_at
)
SELECT DISTINCT ON (c.owner_id, d.language, c.canonical_lemma, c.upos)
 c.owner_id, d.language, c.canonical_lemma, c.upos, c.deck_id, NULL, c.created_at
FROM cards c
JOIN decks d ON d.owner_id = c.owner_id AND d.id = c.deck_id
ORDER BY c.owner_id, d.language, c.canonical_lemma, c.upos, c.created_at, c.id
ON CONFLICT (owner_id, language, canonical_lemma, upos) DO NOTHING;
