-- Installation-wide reference translations (ADR 0116, ticket #2412). A pinned
-- English translation file is parsed once per installation; its cited sections
-- are keyed by the reviewed pin SHA-256 and citation, never by learner. The
-- prepared-deck planner joins them to the cited sections of a representative
-- sentence; a re-import upserts the same citations, so it never changes an
-- already-prepared deck. Different pins coexist.

-- name: UpsertReferenceTranslation :exec
INSERT INTO reference_translations(translation_pin_sha256, language, citation, translation)
VALUES ($1, $2, $3, $4)
ON CONFLICT(translation_pin_sha256, citation) DO UPDATE SET
  language = excluded.language,
  translation = excluded.translation,
  updated_at = now();

-- name: ListReferenceTranslationsByPin :many
SELECT citation, translation
FROM reference_translations
WHERE translation_pin_sha256 = $1
ORDER BY citation;

-- name: HasReferenceTranslations :one
SELECT EXISTS(
  SELECT 1 FROM reference_translations WHERE translation_pin_sha256 = $1
);
