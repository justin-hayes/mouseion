-- Hidden visibility is an owner-and-Book choice stored apart from the
-- Inbox/To Read disposition. A missing row is a visible Book at revision 0.

-- name: GetBookVisibility :one
SELECT COALESCE(v.hidden, false)::boolean AS hidden,
       COALESCE(v.revision, 0)::bigint AS revision
FROM books b
LEFT JOIN book_visibility v ON v.owner_id = b.owner_id AND v.book_id = b.id
WHERE b.owner_id = sqlc.arg('owner') AND b.id = sqlc.arg('book');

-- name: GetBookVisibilityForUpdate :one
SELECT COALESCE(v.hidden, false)::boolean AS hidden,
       COALESCE(v.revision, 0)::bigint AS revision
FROM books b
LEFT JOIN book_visibility v ON v.owner_id = b.owner_id AND v.book_id = b.id
WHERE b.owner_id = sqlc.arg('owner') AND b.id = sqlc.arg('book')
FOR UPDATE OF b;

-- name: WriteBookVisibility :exec
INSERT INTO book_visibility(owner_id, book_id, hidden, revision)
VALUES (sqlc.arg('owner'), sqlc.arg('book'), sqlc.arg('hidden'), sqlc.arg('revision'))
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    hidden = EXCLUDED.hidden,
    revision = EXCLUDED.revision,
    updated_at = now();
