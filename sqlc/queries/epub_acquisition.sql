-- EPUB acquisition failure queries. The row is keyed by owner and Book and
-- holds the latest failed attempt only; a successful acquisition deletes it.

-- name: PutEPUBAcquisitionFailure :exec
INSERT INTO epub_acquisition_failures(owner_id, book_id, connection_id, entry_identifier, stage, code)
VALUES (sqlc.arg('owner_id'), sqlc.arg('book_id'), sqlc.arg('connection_id'), sqlc.arg('entry_identifier'), sqlc.arg('stage'), sqlc.arg('code'))
ON CONFLICT (owner_id, book_id) DO UPDATE SET
  connection_id = EXCLUDED.connection_id,
  entry_identifier = EXCLUDED.entry_identifier,
  stage = EXCLUDED.stage,
  code = EXCLUDED.code,
  failed_at = now();

-- name: DeleteEPUBAcquisitionFailure :exec
DELETE FROM epub_acquisition_failures
WHERE owner_id = sqlc.arg('owner_id') AND book_id = sqlc.arg('book_id');

-- name: ListEPUBAcquisitionFailures :many
SELECT owner_id::text AS owner_id,
       book_id::text AS book_id,
       COALESCE(connection_id::text, ''::text)::text AS connection_id,
       COALESCE(entry_identifier, '') AS entry_identifier,
       stage,
       code,
       failed_at
FROM epub_acquisition_failures
WHERE owner_id = sqlc.arg('owner_id')
  AND book_id = ANY(sqlc.arg('book_ids')::uuid[]);
