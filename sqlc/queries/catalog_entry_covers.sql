-- Retained Catalog entry covers (ADR 0130). The row is keyed by the owner and
-- the entry's connection-scoped identity and has no foreign keys, so every
-- query scopes by owner_id. Bytes are read only by GetCatalogEntryCoverResource.

-- name: ListCatalogEntryCoverStates :many
SELECT entry_identifier, state, (failure_reason IS NOT NULL)::boolean AS failed
FROM catalog_entry_covers
WHERE owner_id = sqlc.arg('owner_id') AND connection_id = sqlc.arg('connection_id');

-- name: AdvertiseCatalogEntryCover :exec
-- Starts a retrieval generation. A retained image stays available until the
-- replacement validates; any other state restarts as pending.
INSERT INTO catalog_entry_covers(owner_id, connection_id, entry_identifier, state, advertised_at)
VALUES (sqlc.arg('owner_id'), sqlc.arg('connection_id'), sqlc.arg('entry_identifier'), 'pending', sqlc.arg('advertised_at'))
ON CONFLICT (owner_id, connection_id, entry_identifier) DO UPDATE SET
  state = CASE WHEN catalog_entry_covers.state = 'available' THEN 'available' ELSE 'pending' END,
  advertised_at = EXCLUDED.advertised_at,
  failure_reason = NULL,
  updated_at = now();

-- name: DeleteCatalogEntryCoversNotAdvertised :execrows
-- Removes the covers of entries that left the index or no longer advertise an
-- image, except one a Book still selects: a Book keeps the image of an entry
-- that left the index (ADR 0077). An entry that explicitly stopped advertising
-- one has already had its Books demoted.
DELETE FROM catalog_entry_covers AS cover
WHERE cover.owner_id = sqlc.arg('owner_id')
  AND cover.connection_id = sqlc.arg('connection_id')
  AND NOT EXISTS (
      SELECT 1
      FROM catalogue_entries AS e
      WHERE e.connection_id = cover.connection_id
        AND e.entry_identifier = cover.entry_identifier
        AND e.cover_href <> ''
  )
  AND NOT EXISTS (
      SELECT 1
      FROM book_covers AS bc
      WHERE bc.owner_id = cover.owner_id
        AND bc.selected_connection_id = cover.connection_id
        AND bc.selected_source_identifier = cover.entry_identifier
  );

-- name: DeleteUnusedCatalogEntryCoversForConnection :exec
-- A deleted connection takes the covers no Book selects; a selected image stays.
DELETE FROM catalog_entry_covers AS cover
WHERE cover.owner_id = sqlc.arg('owner_id')
  AND cover.connection_id = sqlc.arg('connection_id')
  AND NOT EXISTS (
      SELECT 1
      FROM book_covers AS bc
      WHERE bc.owner_id = cover.owner_id
        AND bc.selected_connection_id = cover.connection_id
        AND bc.selected_source_identifier = cover.entry_identifier
  );

-- name: SettlePendingCatalogEntryCoversForConnection :exec
-- A retrieval that can never run again, because its connection is gone, is
-- unavailable rather than pending forever.
UPDATE catalog_entry_covers
SET state = 'unavailable',
    failure_reason = 'cover retrieval source was deleted',
    updated_at = now()
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND state = 'pending';

-- name: ListCatalogEntryCoverGeneration :many
SELECT entry_identifier
FROM catalog_entry_covers
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND advertised_at = sqlc.arg('advertised_at')
ORDER BY entry_identifier;

-- name: GetCatalogEntryCoverForRetrieval :one
SELECT state, advertised_at
FROM catalog_entry_covers
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND entry_identifier = sqlc.arg('entry_identifier');

-- name: SaveCatalogEntryCover :execrows
-- Fenced by the advertisement generation: a stale completion changes nothing.
UPDATE catalog_entry_covers
SET state = 'available',
    media_type = sqlc.arg('media_type'),
    width = sqlc.arg('width'),
    height = sqlc.arg('height'),
    content_hash = sqlc.arg('content_hash'),
    bytes = sqlc.arg('bytes'),
    fetched_at = now(),
    failure_reason = NULL,
    updated_at = now()
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND entry_identifier = sqlc.arg('entry_identifier')
  AND advertised_at = sqlc.arg('advertised_at');

-- name: MarkCatalogEntryCoverUnavailable :execrows
-- A failed retrieval keeps any retained image and records the reason.
UPDATE catalog_entry_covers
SET state = CASE WHEN state = 'available' THEN state ELSE 'unavailable' END,
    failure_reason = sqlc.arg('failure_reason'),
    updated_at = now()
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND entry_identifier = sqlc.arg('entry_identifier')
  AND advertised_at = sqlc.arg('advertised_at');

-- name: GetCatalogEntryCoverResource :one
SELECT media_type, content_hash, width, height, bytes
FROM catalog_entry_covers
WHERE owner_id = sqlc.arg('owner_id')
  AND connection_id = sqlc.arg('connection_id')
  AND entry_identifier = sqlc.arg('entry_identifier')
  AND state = 'available';
