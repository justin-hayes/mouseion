-- Cover metadata is projected into my_books_evidence; bytes are read only by
-- the authenticated cover endpoint.

-- name: GetBookCover :one
SELECT owner_id::text,
       book_id::text,
       state,
       selected_connection_id::text,
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

-- name: RecordBookCoverAdvertisement :exec
INSERT INTO book_covers (owner_id, book_id, state, selected_connection_id, selected_source_identifier, advertised_at, updated_at)
VALUES (sqlc.arg('owner'), sqlc.arg('book'), sqlc.arg('state'), sqlc.arg('connection'), sqlc.arg('source_identifier'), now(), now())
ON CONFLICT (owner_id, book_id) DO UPDATE SET
    state = CASE
        WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.state
        ELSE sqlc.arg('state')
    END,
    selected_connection_id = CASE
        WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.selected_connection_id
        ELSE sqlc.arg('connection')
    END,
    selected_source_identifier = CASE
        WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.selected_source_identifier
        ELSE sqlc.arg('source_identifier')
    END,
    media_type = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.media_type ELSE NULL END,
    width = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.width ELSE NULL END,
    height = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.height ELSE NULL END,
    content_hash = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.content_hash ELSE NULL END,
    bytes = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.bytes ELSE NULL END,
    fetched_at = CASE WHEN book_covers.state = 'available' AND sqlc.arg('state') = 'pending' THEN book_covers.fetched_at ELSE NULL END,
    failure_reason = NULL,
    advertised_at = now(),
    updated_at = now();

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
       AND book_covers.selected_source_identifier IS NOT DISTINCT FROM EXCLUDED.selected_source_identifier);

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
