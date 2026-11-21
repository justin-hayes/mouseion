-- Catalog index queries. A sync reconciles one connection's run of entries in
-- a single transaction: upsert everything it saw, then delete what the
-- complete run did not see. Ownership is checked through the connection.

-- name: LockCatalogueIndexConnection :one
SELECT id::text
FROM opds_connections
WHERE owner_id = sqlc.arg('owner_id') AND id = sqlc.arg('connection_id')
FOR UPDATE;

-- name: UpsertCatalogueEntry :exec
INSERT INTO catalogue_entries(connection_id, entry_identifier, title, author, language_tag, cover_href, cover_media_type, epub_href, last_seen_sync)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (connection_id, entry_identifier) DO UPDATE SET
  title = EXCLUDED.title,
  author = EXCLUDED.author,
  language_tag = EXCLUDED.language_tag,
  cover_href = EXCLUDED.cover_href,
  cover_media_type = EXCLUDED.cover_media_type,
  epub_href = EXCLUDED.epub_href,
  last_seen_sync = EXCLUDED.last_seen_sync;

-- name: DeleteCatalogueEntriesNotSeen :execrows
DELETE FROM catalogue_entries e
USING opds_connections c
WHERE e.connection_id = c.id
  AND c.owner_id = $1
  AND e.connection_id = $2
  AND e.last_seen_sync < $3;

-- name: ListCatalogueEntries :many
SELECT e.connection_id::text AS connection_id, e.entry_identifier, e.title, e.author, e.language_tag, e.cover_href, e.cover_media_type, e.epub_href, e.last_seen_sync
FROM catalogue_entries e
JOIN opds_connections c ON c.id = e.connection_id
WHERE c.owner_id = $1 AND e.connection_id = $2
ORDER BY e.title, e.author, e.entry_identifier;

-- name: GetCatalogueEntry :one
SELECT e.connection_id::text AS connection_id, e.entry_identifier, e.title, e.author, e.language_tag, e.cover_href, e.cover_media_type, e.epub_href, e.last_seen_sync
FROM catalogue_entries e
JOIN opds_connections c ON c.id = e.connection_id
WHERE c.owner_id = $1 AND e.connection_id = $2 AND e.entry_identifier = $3;

-- name: ListCatalogueEntryBooksForConnection :many
-- The learner's active Books linked by catalog-entry alias, keyed by the entry
-- identifier the index browses. A removed Book contributes no ownership.
SELECT a.value AS entry_identifier, a.book_id::text AS book_id
FROM book_aliases a
JOIN book_membership m ON m.owner_id = a.owner_id AND m.book_id = a.book_id AND m.state = 'active'
WHERE a.owner_id = $1 AND a.connection_id = $2
  AND a.alias_type = $3 AND a.namespace = $4;
