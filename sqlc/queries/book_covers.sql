-- A Book's cover resolves through the Catalog entry it selected (ADR 0130).
-- book_covers is only the selection pointer; the image and its retrieval state
-- live in catalog_entry_covers. Cover bytes are read only by the authenticated
-- cover endpoint; every other projection is byte-free.

-- name: GetBookCover :one
SELECT state, width, height
FROM book_cover_states
WHERE owner_id = sqlc.arg('owner') AND book_id = sqlc.arg('book');

-- name: GetBookCoverResource :one
SELECT bc.owner_id::text AS owner_id,
       bc.book_id::text AS book_id,
       c.media_type,
       c.content_hash,
       c.width,
       c.height,
       c.bytes
FROM book_covers bc
JOIN catalog_entry_covers c
  ON c.owner_id = bc.owner_id
 AND c.connection_id = bc.selected_connection_id
 AND c.entry_identifier = bc.selected_source_identifier
 AND c.state = 'available'
WHERE bc.owner_id = sqlc.arg('owner') AND bc.book_id = sqlc.arg('book');

-- name: GetActiveBookCoverResource :one
SELECT bc.owner_id::text AS owner_id,
       bc.book_id::text AS book_id,
       c.media_type,
       c.content_hash,
       c.width,
       c.height,
       c.bytes
FROM book_covers bc
JOIN book_membership m ON m.owner_id = bc.owner_id AND m.book_id = bc.book_id AND m.state = 'active'
JOIN catalog_entry_covers c
  ON c.owner_id = bc.owner_id
 AND c.connection_id = bc.selected_connection_id
 AND c.entry_identifier = bc.selected_source_identifier
 AND c.state = 'available'
WHERE bc.owner_id = sqlc.arg('owner') AND bc.book_id = sqlc.arg('book');

-- name: SelectBookCoversFromEntry :exec
-- First success selects (ADR 0077): every Book aliasing the entry selects it
-- once its image is available, unless it already selects an available image.
-- A selection whose entry has no image yet gives way to an available one.
INSERT INTO book_covers (owner_id, book_id, selected_connection_id, selected_source_identifier, updated_at)
SELECT a.owner_id, a.book_id, a.connection_id, a.value, now()
FROM book_aliases AS a
JOIN catalog_entry_covers AS c
  ON c.owner_id = a.owner_id AND c.connection_id = a.connection_id AND c.entry_identifier = a.value AND c.state = 'available'
WHERE a.owner_id = sqlc.arg('owner')
  AND a.connection_id = sqlc.arg('connection')
  AND a.namespace = 'source_identifier'
  AND a.value = sqlc.arg('entry_identifier')
ON CONFLICT (owner_id, book_id) DO UPDATE
SET selected_connection_id = EXCLUDED.selected_connection_id,
    selected_source_identifier = EXCLUDED.selected_source_identifier,
    updated_at = now()
WHERE NOT EXISTS (
    SELECT 1
    FROM catalog_entry_covers AS current
    WHERE current.owner_id = book_covers.owner_id
      AND current.connection_id = book_covers.selected_connection_id
      AND current.entry_identifier = book_covers.selected_source_identifier
      AND current.state = 'available'
);

-- name: DemoteBookCoversForAbsentEntries :many
-- Explicit absence (ADR 0077): the entry is in the index but advertises no
-- image, so every Book that selected it gives the selection up.
DELETE FROM book_covers AS bc
USING catalogue_entries AS e
WHERE bc.owner_id = sqlc.arg('owner')
  AND bc.selected_connection_id = sqlc.arg('connection')
  AND e.connection_id = bc.selected_connection_id
  AND e.entry_identifier = bc.selected_source_identifier
  AND e.cover_href = ''
RETURNING bc.book_id::text AS book_id;

-- name: SelectFallbackBookCovers :exec
-- Another alias with an available image becomes the demoted Books' fallback.
INSERT INTO book_covers (owner_id, book_id, selected_connection_id, selected_source_identifier, updated_at)
SELECT DISTINCT ON (a.owner_id, a.book_id) a.owner_id, a.book_id, c.connection_id, c.entry_identifier, now()
FROM book_aliases AS a
JOIN catalog_entry_covers AS c
  ON c.owner_id = a.owner_id AND c.connection_id = a.connection_id AND c.entry_identifier = a.value AND c.state = 'available'
WHERE a.owner_id = sqlc.arg('owner')
  AND a.book_id = ANY (sqlc.arg('book_ids')::uuid[])
  AND a.namespace = 'source_identifier'
  AND NOT EXISTS (
      SELECT 1
      FROM catalogue_entries AS e
      WHERE e.connection_id = c.connection_id AND e.entry_identifier = c.entry_identifier AND e.cover_href = ''
  )
ORDER BY a.owner_id, a.book_id, a.created_at, a.id
ON CONFLICT (owner_id, book_id) DO NOTHING;
