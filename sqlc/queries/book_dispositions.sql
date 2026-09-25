-- Book dispositions are owner-scoped learner intent. Legacy workflow
-- mutations synchronize this row until the Journey cutover is complete.

-- name: GetBookDisposition :one
SELECT disposition
FROM book_dispositions
WHERE owner_id = $1 AND book_id = $2;

-- name: ListBookDispositions :many
SELECT book_id::text, disposition
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
    updated_at = now();
