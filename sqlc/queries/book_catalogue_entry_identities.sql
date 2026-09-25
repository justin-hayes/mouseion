
-- Connection-independent catalogue identity preserves a Book when a source
-- connection is deleted and later recreated with the same URL.

-- name: GetBookCatalogueEntryIdentityForConnection :one
SELECT i.book_id::text
FROM book_catalogue_entry_identities i
JOIN opds_connections c ON c.owner_id = i.owner_id AND c.url = i.connection_url
WHERE c.owner_id = i.owner_id AND c.name = i.connection_name
  AND i.owner_id = $1 AND c.id = $2 AND i.source_identifier = $3
FOR UPDATE OF i;

-- name: InsertBookCatalogueEntryIdentity :exec
INSERT INTO book_catalogue_entry_identities(owner_id, book_id, connection_url, connection_name, source_identifier)
SELECT $1, $2, c.url, c.name, $4
FROM opds_connections c
WHERE c.owner_id = $1 AND c.id = $3
ON CONFLICT (owner_id, connection_url, connection_name, source_identifier) DO NOTHING;

-- name: PreserveBookCatalogueEntryIdentitiesForConnection :exec
INSERT INTO book_catalogue_entry_identities(owner_id, book_id, connection_url, connection_name, source_identifier)
SELECT a.owner_id, a.book_id, c.url, c.name, a.value
FROM book_aliases a
JOIN opds_connections c ON c.owner_id = a.owner_id AND c.id = a.connection_id
WHERE a.owner_id = $1 AND a.connection_id = $2 AND a.alias_type = 'catalog_entry'
ON CONFLICT (owner_id, connection_url, connection_name, source_identifier) DO NOTHING;

-- name: BookCatalogueEntryIdentityConflictForConnection :one
SELECT EXISTS (
    SELECT 1
    FROM book_aliases a
    JOIN opds_connections c ON c.owner_id = a.owner_id AND c.id = a.connection_id
    JOIN book_catalogue_entry_identities i
      ON i.owner_id = a.owner_id
     AND i.connection_url = c.url
     AND i.connection_name = c.name
     AND i.source_identifier = a.value
    WHERE a.owner_id = $1 AND a.connection_id = $2 AND i.book_id <> a.book_id
);
