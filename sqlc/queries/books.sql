-- Book identity and My Books evidence queries. The read models select from
-- the my_books_evidence view (migration 000066, rebuilt over the shared
-- source_material_evidence view in 000068) so the composed projection and its
-- analysis status/state classification are one SQL artifact instead of Go
-- string stitching.

-- name: ListActiveBooks :many
SELECT b.id::text,
       b.owner_id::text,
       b.title,
       b.author,
       b.metadata_provenance,
       b.language_state,
       COALESCE(b.language_tag, '') AS language_tag,
       b.created_at,
       b.updated_at
FROM books b
JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
WHERE b.owner_id = sqlc.arg('owner')
ORDER BY b.title, b.id;

-- name: GetBookForUpdate :one
SELECT id::text
FROM books
WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: ListMyBooksEvidence :many
SELECT * FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner')
ORDER BY book_title, book_id;

-- name: BrowseMyBooksEvidence :many
SELECT * FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner')
  AND (sqlc.arg('query')::text = '' OR lower(book_title) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  )
ORDER BY lower(book_title), book_title, book_id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountMyBooksFiltered :one
SELECT count(*) FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner')
  AND (sqlc.arg('query')::text = '' OR lower(book_title) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  );

-- name: CountMyBooksScope :one
SELECT count(*) FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  );

-- name: CountMyBooksAll :one
SELECT count(*) FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner');

-- name: CountMyBooksByLanguage :many
SELECT (CASE WHEN book_language_state = 'unknown' THEN 'unknown' ELSE book_language_tag END)::text AS language_tag,
       count(*) AS book_count
FROM my_books_evidence
WHERE book_owner_id = sqlc.arg('owner') AND book_language_state IN ('chosen', 'unknown')
GROUP BY 1;

-- name: GetMyBookDetail :one
SELECT e.* FROM my_books_evidence e
WHERE e.book_owner_id = sqlc.arg('owner') AND (e.book_id = sqlc.arg('id') OR EXISTS (
  SELECT 1 FROM source_materials requested_source
  WHERE requested_source.owner_id::text = e.book_owner_id
    AND requested_source.book_id::text = e.book_id
    AND requested_source.id::text = sqlc.arg('id')
))
ORDER BY CASE WHEN e.book_id = sqlc.arg('id') THEN 0 ELSE 1 END
LIMIT 1;
