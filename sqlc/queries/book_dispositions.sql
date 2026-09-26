-- Book dispositions are owner-scoped learner intent. Legacy workflow
-- mutations synchronize this row until the Journey cutover is complete.

-- name: GetBookDisposition :one
SELECT disposition
FROM book_dispositions
WHERE owner_id = $1 AND book_id = $2;

-- name: GetBookDispositionStateForUpdate :one
SELECT disposition, revision
FROM book_dispositions
WHERE owner_id = $1 AND book_id = $2
FOR UPDATE;

-- name: ListBookDispositions :many
SELECT book_id::text, disposition, revision
FROM book_dispositions
WHERE owner_id = $1;

-- name: InsertInboxBookDisposition :exec
INSERT INTO book_dispositions(owner_id, book_id, disposition)
VALUES ($1, $2, 'inbox')
ON CONFLICT (owner_id, book_id) DO NOTHING;

-- name: UpsertBookDisposition :exec
INSERT INTO book_dispositions(owner_id, book_id, disposition)
VALUES ($1, $2, $3)
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    disposition = EXCLUDED.disposition,
    revision = book_dispositions.revision + 1,
    updated_at = now();

-- name: TransitionBookDisposition :execrows
UPDATE book_dispositions
SET disposition = sqlc.arg('disposition'),
    revision = CASE WHEN revision = sqlc.arg('expected_revision') THEN revision + 1 ELSE revision END,
    updated_at = CASE WHEN revision = sqlc.arg('expected_revision') THEN now() ELSE updated_at END
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
  AND (revision = sqlc.arg('expected_revision') OR
       (revision = sqlc.arg('expected_revision') + 1 AND disposition = sqlc.arg('disposition')));
