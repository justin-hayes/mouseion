-- Book identity and My Books evidence queries. The read models select from
-- the my_books_evidence view (migration 000066, rebuilt over the shared
-- source_material_evidence view in 000068) so the composed projection and its
-- analysis status/state classification are one SQL artifact instead of Go
-- string stitching.

-- name: BookExists :one
SELECT EXISTS(
  SELECT 1 FROM books
  WHERE owner_id = sqlc.arg('owner') AND id = sqlc.arg('book')
);

-- name: ResolveLinkedSourceBook :one
SELECT (COALESCE(sm.book_id::text, b.id::text, ''))::text AS linked_book_id
FROM source_materials sm
LEFT JOIN book_aliases a
  ON a.owner_id = sm.owner_id
 AND a.namespace = sqlc.arg('namespace')
 AND a.value = sm.source_identifier
LEFT JOIN books b ON b.owner_id = a.owner_id AND b.id = a.book_id
WHERE sm.owner_id = sqlc.arg('owner') AND sm.id = sqlc.arg('source');

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
SELECT e.*
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner')
ORDER BY e.book_title, e.book_id;

-- name: BrowseMyBooksEvidence :many
SELECT e.*
     , COALESCE(history.completion_count, 0)::bigint AS completion_count
     , history.latest_completed_at
     , COALESCE(history.latest_completion_source, '')::text AS latest_completion_source
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
LEFT JOIN LATERAL (
    SELECT count(*) AS completion_count,
           COALESCE((SELECT h.completed_at FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1), 'epoch'::timestamptz) AS latest_completed_at,
           (SELECT h.completion_source FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id ORDER BY h.completed_at DESC, h.completion_id DESC LIMIT 1) AS latest_completion_source
    FROM reading_history h
    WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
) history ON true
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (sqlc.arg('query')::text = '' OR lower(book_title) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\' OR lower(book_author) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  )
  AND (sqlc.arg('disposition')::text = '' OR d.disposition = sqlc.arg('disposition'))
  AND (NOT sqlc.arg('history')::boolean OR EXISTS (
    SELECT 1 FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
  ))
ORDER BY lower(e.book_title), e.book_title, e.book_id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountMyBooksFiltered :one
SELECT count(*) FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (sqlc.arg('query')::text = '' OR lower(book_title) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\' OR lower(book_author) LIKE '%' || sqlc.arg('query') || '%' ESCAPE '\')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND book_language_state = 'unknown')
    OR (book_language_state = 'chosen' AND book_language_tag = sqlc.arg('language'))
  )
  AND (sqlc.arg('disposition')::text = '' OR d.disposition = sqlc.arg('disposition'))
  AND (NOT sqlc.arg('history')::boolean OR EXISTS (
    SELECT 1 FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
  ));

-- name: CountMyBooksScope :one
SELECT count(*) FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner')
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

-- name: CountMyBooksByDisposition :many
SELECT d.disposition, count(*) AS book_count
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND e.book_language_state = 'unknown')
    OR (e.book_language_state = 'chosen' AND e.book_language_tag = sqlc.arg('language'))
  )
GROUP BY d.disposition;

-- name: CountMyBooksWithHistory :one
SELECT count(*) FROM my_books_evidence e
WHERE e.book_owner_id = sqlc.arg('owner')
  AND (
    sqlc.arg('language')::text = ''
    OR (sqlc.arg('language')::text = 'unknown' AND e.book_language_state = 'unknown')
    OR (e.book_language_state = 'chosen' AND e.book_language_tag = sqlc.arg('language'))
  )
  AND EXISTS (
    SELECT 1 FROM reading_history h WHERE h.owner_id::text = e.book_owner_id AND h.book_id::text = e.book_id
  );

-- name: GetMyBookDetail :one
SELECT e.*
FROM my_books_evidence e
JOIN book_dispositions d ON d.owner_id::text = e.book_owner_id AND d.book_id::text = e.book_id
WHERE e.book_owner_id = sqlc.arg('owner') AND (e.book_id = sqlc.arg('id') OR EXISTS (
  SELECT 1 FROM source_materials requested_source
  WHERE requested_source.owner_id::text = e.book_owner_id
    AND requested_source.book_id::text = e.book_id
    AND requested_source.id::text = sqlc.arg('id')
))
ORDER BY CASE WHEN e.book_id = sqlc.arg('id') THEN 0 ELSE 1 END
LIMIT 1;
