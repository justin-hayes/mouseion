-- Operator-imported reference translations (Brownson's Anabasis translation in
-- the Perseus perseus-eng2 digitisation), keyed by citation. The prepared-deck
-- planner joins them to the cited sections of a representative sentence; a
-- re-import upserts the same citations, so it never changes an already-prepared
-- deck.

-- name: UpsertReferenceTranslation :exec
INSERT INTO reference_translations(owner_id, language, citation, translation, source_pin)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(owner_id, language, citation) DO UPDATE SET
  translation = excluded.translation,
  source_pin = excluded.source_pin,
  updated_at = now();

-- name: ListReferenceTranslations :many
SELECT citation, translation, source_pin
FROM reference_translations
WHERE owner_id = $1 AND language = $2
ORDER BY citation;
