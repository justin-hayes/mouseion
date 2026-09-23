-- Cover metadata is projected into my_books_evidence; bytes are read only by
-- the authenticated cover endpoint. The selected source is the Catalog entry
-- that owns the retained display image; only that source may replace or
-- explicitly remove it. While no image is selected, another alias may become
-- the candidate that the next successful retrieval selects.

-- name: GetBookCover :one
SELECT owner_id::text,
       book_id::text,
       state,
       (COALESCE(selected_connection_id::text, ''))::text AS selected_connection_id,
       selected_source_identifier,
       media_type,
       width,
       height,
       content_hash,
       bytes,
       advertised_at,
       fetched_at,
       updated_at,
       failure_reason
FROM book_covers
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: GetBookCoverForUpdate :one
SELECT state,
       (COALESCE(selected_connection_id::text, ''))::text AS selected_connection_id,
       (COALESCE(selected_source_identifier, ''))::text AS selected_source_identifier
FROM book_covers
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
FOR UPDATE;

-- name: GetBookCoverForRetrieval :one
SELECT state,
       (COALESCE(selected_connection_id::text, ''))::text AS selected_connection_id,
       (COALESCE(selected_source_identifier, ''))::text AS selected_source_identifier,
       (COALESCE(failure_reason, ''))::text AS failure_reason
FROM book_covers
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: InsertBookCoverCandidate :exec
INSERT INTO book_covers (owner_id, book_id, state, selected_connection_id, selected_source_identifier, advertised_at, updated_at)
VALUES (
    sqlc.arg('owner'),
    sqlc.arg('book'),
    sqlc.arg('state'),
    CASE WHEN sqlc.arg('advertised') THEN sqlc.arg('connection')::uuid ELSE NULL END,
    CASE WHEN sqlc.arg('advertised') THEN sqlc.arg('source_identifier')::text ELSE NULL END,
    now(),
    now()
);

-- name: SetBookCoverPending :exec
UPDATE book_covers
SET state = 'pending',
    selected_connection_id = sqlc.arg('connection'),
    selected_source_identifier = sqlc.arg('source_identifier'),
    media_type = NULL,
    width = NULL,
    height = NULL,
    content_hash = NULL,
    bytes = NULL,
    fetched_at = NULL,
    failure_reason = NULL,
    advertised_at = now(),
    updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: RefreshBookCoverAdvertisement :exec
UPDATE book_covers
SET advertised_at = now(), failure_reason = NULL, updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book') AND state = 'available';

-- name: ClearBookCover :exec
UPDATE book_covers
SET state = 'none',
    selected_connection_id = NULL,
    selected_source_identifier = NULL,
    media_type = NULL,
    width = NULL,
    height = NULL,
    content_hash = NULL,
    bytes = NULL,
    fetched_at = NULL,
    failure_reason = NULL,
    updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: SaveBookCover :exec
INSERT INTO book_covers (owner_id, book_id, state, selected_connection_id, selected_source_identifier, media_type, width, height, content_hash, bytes, advertised_at, fetched_at, updated_at)
VALUES (sqlc.arg('owner'), sqlc.arg('book'), 'available', sqlc.arg('connection'), sqlc.arg('source_identifier'), sqlc.arg('media_type'), sqlc.arg('width'), sqlc.arg('height'), sqlc.arg('content_hash'), sqlc.arg('bytes'), now(), now(), now())
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    state = 'available',
    selected_connection_id = EXCLUDED.selected_connection_id,
    selected_source_identifier = EXCLUDED.selected_source_identifier,
    media_type = EXCLUDED.media_type,
    width = EXCLUDED.width,
    height = EXCLUDED.height,
    content_hash = EXCLUDED.content_hash,
    bytes = EXCLUDED.bytes,
    fetched_at = now(),
    failure_reason = NULL,
    updated_at = now()
WHERE book_covers.state <> 'available'
   OR (book_covers.selected_connection_id IS NOT DISTINCT FROM EXCLUDED.selected_connection_id
       AND book_covers.selected_source_identifier IS NOT DISTINCT FROM EXCLUDED.selected_source_identifier
       AND book_covers.content_hash IS DISTINCT FROM EXCLUDED.content_hash);

-- name: MarkBookCoverUnavailable :exec
UPDATE book_covers
SET state = CASE WHEN state = 'available' THEN state ELSE 'unavailable' END,
    bytes = CASE WHEN state = 'available' THEN bytes ELSE NULL END,
    media_type = CASE WHEN state = 'available' THEN media_type ELSE NULL END,
    width = CASE WHEN state = 'available' THEN width ELSE NULL END,
    height = CASE WHEN state = 'available' THEN height ELSE NULL END,
    content_hash = CASE WHEN state = 'available' THEN content_hash ELSE NULL END,
    failure_reason = sqlc.arg('failure_reason'), updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');
