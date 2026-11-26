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

-- name: GetBookTranslationPin :one
-- The pinned translation file a Book's reading unit imported its passage
-- translations from, empty when it has none. Study reads it to freeze a
-- representative sentence's passage with the word's material (ADR 0138).
SELECT COALESCE(ru.translation_file_sha256, '')::text AS pin
FROM book_reading_units bru
JOIN reading_units ru ON ru.id = bru.reading_unit_id
WHERE bru.owner_id = sqlc.arg('owner') AND bru.book_id = sqlc.arg('book')
  AND COALESCE(ru.translation_file_sha256, '') <> ''
ORDER BY ru.id
LIMIT 1;
