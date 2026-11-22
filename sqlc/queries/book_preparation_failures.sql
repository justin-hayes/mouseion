-- One Book preparation failure record (ADR 0121). Classical preparation and
-- EPUB acquisition share the table, key, stage and code vocabulary; they differ
-- only in provenance. Each kind reads and writes only its own rows so clearing
-- one kind never removes the other's evidence.

-- name: PutClassicalPreparationFailure :exec
INSERT INTO book_preparation_failures(
    owner_id, book_id, reading_unit_key, definition_revision, stage, code)
VALUES (
    sqlc.arg('owner_id'), sqlc.arg('book_id'), sqlc.arg('reading_unit_key'),
    sqlc.arg('definition_revision'), sqlc.arg('stage'), sqlc.arg('code'))
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    reading_unit_key = EXCLUDED.reading_unit_key,
    definition_revision = EXCLUDED.definition_revision,
    connection_id = NULL,
    entry_identifier = NULL,
    stage = EXCLUDED.stage,
    code = EXCLUDED.code,
    failed_at = now();

-- name: GetClassicalPreparationFailure :one
SELECT owner_id::text AS owner_id,
       book_id::text AS book_id,
       reading_unit_key,
       definition_revision,
       stage,
       code
FROM book_preparation_failures
WHERE owner_id = sqlc.arg('owner_id')
  AND book_id = sqlc.arg('book_id')
  AND reading_unit_key IS NOT NULL;

-- name: DeleteClassicalPreparationFailure :exec
DELETE FROM book_preparation_failures
WHERE owner_id = sqlc.arg('owner_id')
  AND book_id = sqlc.arg('book_id')
  AND reading_unit_key IS NOT NULL;

-- name: PutEPUBAcquisitionFailure :exec
INSERT INTO book_preparation_failures(
    owner_id, book_id, connection_id, entry_identifier, stage, code)
VALUES (
    sqlc.arg('owner_id'), sqlc.arg('book_id'), sqlc.arg('connection_id'),
    sqlc.arg('entry_identifier'), sqlc.arg('stage'), sqlc.arg('code'))
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    reading_unit_key = NULL,
    definition_revision = NULL,
    connection_id = EXCLUDED.connection_id,
    entry_identifier = EXCLUDED.entry_identifier,
    stage = EXCLUDED.stage,
    code = EXCLUDED.code,
    failed_at = now();

-- name: DeleteEPUBAcquisitionFailure :exec
DELETE FROM book_preparation_failures
WHERE owner_id = sqlc.arg('owner_id')
  AND book_id = sqlc.arg('book_id')
  AND reading_unit_key IS NULL;

-- name: ListEPUBAcquisitionFailures :many
SELECT owner_id::text AS owner_id,
       book_id::text AS book_id,
       COALESCE(connection_id::text, ''::text)::text AS connection_id,
       COALESCE(entry_identifier, '') AS entry_identifier,
       stage,
       code,
       failed_at
FROM book_preparation_failures
WHERE owner_id = sqlc.arg('owner_id')
  AND book_id = ANY(sqlc.arg('book_ids')::uuid[])
  AND reading_unit_key IS NULL;
