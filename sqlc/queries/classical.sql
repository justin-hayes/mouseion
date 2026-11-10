-- Classical reading units and their Book association. Definitions are shared
-- reference data upserted from reviewed application values; the association is
-- insert-only. Domain-level identity and refusal decisions remain in
-- persistence and the ancientgreek service.

-- name: UpsertWork :one
INSERT INTO works(definition_key, title, author)
VALUES ($1, $2, $3)
ON CONFLICT (definition_key) DO UPDATE SET title = EXCLUDED.title, author = EXCLUDED.author
RETURNING id::text, definition_key, title, author, created_at;

-- name: UpsertEdition :one
INSERT INTO editions(work_id, definition_key, label)
VALUES ($1, $2, $3)
ON CONFLICT (definition_key) DO UPDATE SET work_id = EXCLUDED.work_id, label = EXCLUDED.label
RETURNING id::text, work_id::text, definition_key, label, created_at;

-- name: UpsertReadingUnit :one
INSERT INTO reading_units(edition_id, definition_key, citation_start, citation_end, source_file_name, source_file_size, source_file_sha256, interpretation_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (definition_key) DO UPDATE SET
    edition_id = EXCLUDED.edition_id,
    citation_start = EXCLUDED.citation_start,
    citation_end = EXCLUDED.citation_end,
    source_file_name = EXCLUDED.source_file_name,
    source_file_size = EXCLUDED.source_file_size,
    source_file_sha256 = EXCLUDED.source_file_sha256,
    interpretation_version = EXCLUDED.interpretation_version
RETURNING id::text, edition_id::text, definition_key, citation_start, citation_end,
          source_file_name, source_file_size, source_file_sha256, interpretation_version, created_at;

-- name: GetRegisteredBookID :one
SELECT book_id::text
FROM book_reading_units
WHERE owner_id = $1 AND reading_unit_id = $2;

-- name: GetCurrentClassicalUnitByKey :one
-- The current, non-withdrawn Reading unit a catalog Add names, with the Work
-- metadata the new Book copies. A withdrawn unit is never addable.
SELECT ru.id::text AS id,
       ru.definition_key AS unit_key,
       ru.constituent_book,
       w.title AS work_title,
       w.author AS work_author
FROM reading_units ru
JOIN editions e ON e.id = ru.edition_id
JOIN works w ON w.id = e.work_id
WHERE ru.definition_key = $1 AND ru.withdrawn = false;

-- name: ListOwnedClassicalReadingUnits :many
-- The learner's active Books associated with a Reading unit. The catalog uses
-- this to show "In My Books" without exposing preparation state.
SELECT ru.definition_key AS unit_key,
       bru.book_id::text AS book_id
FROM book_reading_units bru
JOIN reading_units ru ON ru.id = bru.reading_unit_id
JOIN book_membership m ON m.owner_id = bru.owner_id AND m.book_id = bru.book_id AND m.state = 'active'
WHERE bru.owner_id = $1
ORDER BY ru.constituent_book NULLS FIRST, ru.definition_key;

-- name: InsertBookReadingUnit :exec
INSERT INTO book_reading_units(owner_id, book_id, reading_unit_id)
VALUES ($1, $2, $3);

-- name: ListBookReadingUnits :many
SELECT bru.book_id::text AS book_id,
       bru.reading_unit_id::text AS reading_unit_id,
       ru.definition_key AS reading_unit_key,
       ru.citation_start,
       ru.citation_end,
       e.label AS edition_label,
       w.definition_key AS work_key,
       w.title AS work_title,
       w.author AS work_author,
       w.greek_title,
       ru.constituent_book,
       ru.constituent_books,
       ru.revision,
       ru.withdrawn
FROM book_reading_units bru
JOIN reading_units ru ON ru.id = bru.reading_unit_id
JOIN editions e ON e.id = ru.edition_id
JOIN works w ON w.id = e.work_id
WHERE bru.owner_id = sqlc.arg('owner') AND bru.book_id = ANY(sqlc.arg('book_ids')::uuid[]);

-- name: ListLegacyLanguageRows :many
-- Rows carrying a language identity that predate the classical registration.
-- A Book already associated with a Reading unit, and the source materials
-- pointing at it, are this feature's own data and are not legacy. A learner's
-- first registration passes only when they own no such rows, so every derived
-- row (tokens, Known, vocabulary, candidates, decks) a registered learner owns
-- was written by this feature: an operator re-import is not refused by them.
WITH registered_owners AS (
    SELECT DISTINCT bru.owner_id
    FROM book_reading_units bru
    JOIN books b ON b.owner_id = bru.owner_id AND b.id = bru.book_id
    WHERE b.language_tag = $1
)
SELECT t.owner_id, t.surface, count(*)::bigint AS row_count
FROM (
    SELECT b.owner_id::text AS owner_id, 'books'::text AS surface
    FROM books b
    WHERE b.language_tag = $1
      AND NOT EXISTS (
          SELECT 1 FROM book_reading_units bru
          WHERE bru.owner_id = b.owner_id AND bru.book_id = b.id
      )
    UNION ALL
    SELECT sm.owner_id::text, 'source_materials'::text
    FROM source_materials sm
    WHERE sm.language = $1
      AND NOT EXISTS (
          SELECT 1 FROM book_reading_units bru
          WHERE bru.owner_id = sm.owner_id AND bru.book_id = sm.book_id
      )
    UNION ALL
    -- A pre-upgrade classical Book keeps the whole work file under the source
    -- file name until startup conversion rewrites it to the Reading-unit key.
    -- Such a Book is legacy even though it already has a Reading-unit link, so
    -- Add and preparation stay fail-closed until the conversion completes.
    SELECT sm.owner_id::text, 'source_materials'::text
    FROM source_materials sm
    JOIN book_reading_units bru ON bru.owner_id = sm.owner_id AND bru.book_id = sm.book_id
    JOIN reading_units ru ON ru.id = bru.reading_unit_id
    LEFT JOIN classical_prepared_definitions p
      ON p.owner_id = sm.owner_id AND p.book_id = sm.book_id
    WHERE sm.language = $1
      AND sm.media_type = $2
      AND sm.source_identifier <> ru.definition_key
      AND p.book_id IS NULL
    UNION ALL
    SELECT ct.owner_id::text, 'corpus_tokens'::text
    FROM corpus_tokens ct
    WHERE ct.language = $1
      AND ct.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT pg.owner_id::text, 'primary_goals'::text
    FROM primary_goals pg
    WHERE pg.language = $1
      AND NOT EXISTS (
          SELECT 1 FROM book_reading_units bru
          WHERE bru.owner_id = pg.owner_id AND bru.book_id = pg.book_id
      )
    UNION ALL
    SELECT kv.owner_id::text, 'known_vocabulary'::text
    FROM known_vocabulary kv
    WHERE kv.language = $1
      AND kv.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT vs.owner_id::text, 'vocabulary_states'::text
    FROM vocabulary_states vs
    WHERE vs.language = $1
      AND vs.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT gv.owner_id::text, 'generated_vocabulary'::text
    FROM generated_vocabulary gv
    WHERE gv.language = $1
      AND gv.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT sc.owner_id::text, 'selection_candidates'::text
    FROM selection_candidates sc
    WHERE sc.language = $1
      AND sc.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT d.owner_id::text, 'decks'::text
    FROM decks d
    WHERE d.language = $1
      AND d.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT dpv.owner_id::text, 'deck_preparation_vocabulary'::text
    FROM deck_preparation_vocabulary dpv
    WHERE dpv.language = $1
      AND dpv.owner_id NOT IN (SELECT owner_id FROM registered_owners)
    UNION ALL
    SELECT dpmi.owner_id::text, 'deck_preparation_manifest_items'::text
    FROM deck_preparation_manifest_items dpmi
    WHERE dpmi.language = $1
      AND dpmi.owner_id NOT IN (SELECT owner_id FROM registered_owners)
) t
GROUP BY t.owner_id, t.surface
ORDER BY t.owner_id, t.surface;
