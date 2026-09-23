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

-- name: GetActiveBookCoverResource :one
SELECT c.owner_id::text,
       c.book_id::text,
       c.state,
       (COALESCE(c.selected_connection_id::text, ''))::text AS selected_connection_id,
       c.selected_source_identifier,
       c.media_type,
       c.width,
       c.height,
       c.content_hash,
       c.bytes,
       c.advertised_at,
       c.fetched_at,
       c.updated_at,
       c.failure_reason
FROM book_covers c
JOIN book_membership m ON m.owner_id = c.owner_id AND m.book_id = c.book_id AND m.state = 'active'
WHERE c.owner_id = sqlc.arg('owner') AND c.book_id = sqlc.arg('book');

-- name: GetBookCoverForUpdate :one
SELECT state,
       (COALESCE(selected_connection_id::text, ''))::text AS selected_connection_id,
       (COALESCE(selected_source_identifier, ''))::text AS selected_source_identifier,
       advertised_at
FROM book_covers
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book')
FOR UPDATE;

-- name: GetBookCoverForRetrieval :one
SELECT state,
       (COALESCE(selected_connection_id::text, ''))::text AS selected_connection_id,
       (COALESCE(selected_source_identifier, ''))::text AS selected_source_identifier,
       (COALESCE(failure_reason, ''))::text AS failure_reason,
       advertised_at
FROM book_covers
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: InsertBookCoverCandidate :execrows
INSERT INTO book_covers (owner_id, book_id, state, selected_connection_id, selected_source_identifier, advertised_at, updated_at)
VALUES (
    sqlc.arg('owner'),
    sqlc.arg('book'),
    sqlc.arg('state'),
    CASE WHEN sqlc.arg('advertised') THEN sqlc.arg('connection')::uuid ELSE NULL END,
    CASE WHEN sqlc.arg('advertised') THEN sqlc.arg('source_identifier')::text ELSE NULL END,
    now(),
    now()
)
ON CONFLICT (owner_id, book_id) DO NOTHING;

-- name: RegisterBookCoverCandidate :execrows
INSERT INTO book_cover_candidates (owner_id, book_id, connection_id, source_identifier, advertised_at, state, updated_at)
VALUES (sqlc.arg('owner'), sqlc.arg('book'), sqlc.arg('connection'), sqlc.arg('source_identifier'), sqlc.arg('advertised_at'), 'pending', now())
ON CONFLICT (owner_id, book_id, connection_id, source_identifier, advertised_at) DO NOTHING;

-- name: MarkBookCoverCandidateSucceeded :exec
UPDATE book_cover_candidates
SET state = 'succeeded', failure_reason = NULL, updated_at = now()
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND connection_id = sqlc.arg('connection')
  AND source_identifier = sqlc.arg('source_identifier')
  AND advertised_at = sqlc.arg('advertised_at');

-- name: MarkBookCoverCandidateFailed :execrows
UPDATE book_cover_candidates
SET state = 'failed', failure_reason = sqlc.arg('failure_reason'), updated_at = now()
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND connection_id = sqlc.arg('connection')
  AND source_identifier = sqlc.arg('source_identifier')
  AND advertised_at = sqlc.arg('advertised_at')
  AND state = 'pending';

-- name: CountPendingBookCoverCandidates :one
SELECT count(*)::int
FROM book_cover_candidates
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND advertised_at = sqlc.arg('advertised_at')
  AND state = 'pending';

-- name: RecordBookCoverGenerationFailure :exec
UPDATE book_covers
SET failure_reason = sqlc.arg('failure_reason'), updated_at = now()
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND advertised_at = sqlc.arg('advertised_at')
  AND state = 'pending';

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
SET failure_reason = NULL,
    advertised_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND state IN ('available', 'pending');

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
    advertised_at = NULL,
    failure_reason = NULL,
    updated_at = now()
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: SaveBookCover :exec
UPDATE book_covers
SET state = 'available',
    selected_connection_id = sqlc.arg('connection'),
    selected_source_identifier = sqlc.arg('source_identifier'),
    media_type = sqlc.arg('media_type'),
    width = sqlc.arg('width'),
    height = sqlc.arg('height'),
    content_hash = sqlc.arg('content_hash'),
    bytes = sqlc.arg('bytes'),
    fetched_at = now(),
    failure_reason = NULL,
    updated_at = now()
WHERE owner_id = sqlc.arg('owner')
  AND book_id = sqlc.arg('book')
  AND advertised_at IS NOT DISTINCT FROM sqlc.arg('advertised_at')
  AND (state IN ('pending', 'unavailable')
       OR (selected_connection_id = sqlc.arg('connection')
           AND selected_source_identifier = sqlc.arg('source_identifier')))
  AND (state <> 'available' OR content_hash IS DISTINCT FROM sqlc.arg('content_hash'));

-- name: MarkBookCoverUnavailable :exec
UPDATE book_covers
SET state = CASE WHEN state = 'available' THEN state ELSE 'unavailable' END,
    bytes = CASE WHEN state = 'available' THEN bytes ELSE NULL END,
    media_type = CASE WHEN state = 'available' THEN media_type ELSE NULL END,
    width = CASE WHEN state = 'available' THEN width ELSE NULL END,
    height = CASE WHEN state = 'available' THEN height ELSE NULL END,
    content_hash = CASE WHEN state = 'available' THEN content_hash ELSE NULL END,
    failure_reason = sqlc.arg('failure_reason'), updated_at = now()
 WHERE owner_id = sqlc.arg('owner')
   AND book_id = sqlc.arg('book')
   AND selected_connection_id = sqlc.arg('connection')
   AND selected_source_identifier = sqlc.arg('source_identifier')
   AND advertised_at IS NOT DISTINCT FROM sqlc.arg('advertised_at');
