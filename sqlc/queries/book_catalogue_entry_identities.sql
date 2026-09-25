
-- Connection-independent catalogue identity preserves a Book when a source
-- connection is deleted and later recreated with the same URL.

-- name: GetBookCatalogueEntryIdentityForConnection :one
SELECT i.book_id::text
FROM book_catalogue_entry_identities i
JOIN opds_connections c ON c.owner_id = i.owner_id AND c.url = i.connection_url
WHERE i.owner_id = $1 AND c.id = $2 AND i.source_identifier = $3
FOR UPDATE OF i;

-- name: InsertBookCatalogueEntryIdentity :exec
INSERT INTO book_catalogue_entry_identities(owner_id, book_id, connection_url, source_identifier)
SELECT $1, $2, c.url, $4
FROM opds_connections c
WHERE c.owner_id = $1 AND c.id = $3
ON CONFLICT (owner_id, connection_url, source_identifier) DO NOTHING;

-- name: PreserveBookCatalogueEntryIdentitiesForConnection :exec
INSERT INTO book_catalogue_entry_identities(owner_id, book_id, connection_url, source_identifier)
SELECT a.owner_id, a.book_id, c.url, a.value
FROM book_aliases a
JOIN opds_connections c ON c.owner_id = a.owner_id AND c.id = a.connection_id
WHERE a.owner_id = $1 AND a.connection_id = $2 AND a.alias_type = 'catalog_entry'
ON CONFLICT (owner_id, connection_url, source_identifier) DO NOTHING;
