-- Book, membership, and alias identity queries. Domain-level conflict and
-- ownership decisions remain in the persistence methods.

-- name: ListStudyLanguages :many
WITH chosen_languages AS (
  SELECT DISTINCT b.language_tag AS language
  FROM books b
  JOIN book_membership m ON m.owner_id = b.owner_id AND m.book_id = b.id AND m.state = 'active'
  WHERE b.owner_id = $1 AND b.language_state = 'chosen' AND b.language_tag <> ''
)
SELECT c.language, COALESCE(NULLIF(s.display_name, ''), c.language) AS display_name
FROM chosen_languages c
LEFT JOIN supported_languages s ON s.language = c.language
ORDER BY COALESCE(NULLIF(s.display_name, ''), c.language), c.language;

-- name: GetBook :one
SELECT id::text, owner_id::text, title, metadata_provenance, language_state,
       COALESCE(language_tag, '') AS language_tag, created_at, updated_at
FROM books WHERE owner_id = $1 AND id = $2;

-- name: GetBookCatalogEntryAlias :one
SELECT a.id::text, a.owner_id::text, a.book_id::text, (COALESCE(a.connection_id::text, ''))::text AS connection_id,
       a.alias_type, a.namespace, a.value, a.created_at
FROM book_aliases a
JOIN books b ON b.owner_id = a.owner_id AND b.id = a.book_id
JOIN book_membership m ON m.owner_id = a.owner_id AND m.book_id = a.book_id AND m.state = 'active'
WHERE a.owner_id = $1 AND a.book_id = $2 AND a.alias_type = $3 AND a.namespace = $4;

-- name: ListUnscopedCatalogueEntryAliases :many
SELECT id::text, owner_id::text, book_id::text, (COALESCE(connection_id::text, ''))::text AS connection_id,
       alias_type, namespace, value, created_at
FROM book_aliases
WHERE connection_id IS NULL AND alias_type = $1 AND namespace = $2
ORDER BY owner_id, id;

-- name: SetCatalogueEntryAliasConnection :execrows
UPDATE book_aliases a
SET connection_id = $3
WHERE a.owner_id = $1 AND a.id = $2 AND a.connection_id IS NULL
  AND a.alias_type = $4 AND a.namespace = $5
  AND EXISTS (SELECT 1 FROM opds_connections c WHERE c.owner_id = $1 AND c.id = $3);

-- name: GetBookAliasConnection :one
SELECT (COALESCE(connection_id::text, ''))::text AS connection_id
FROM book_aliases WHERE owner_id = $1 AND id = $2;

-- name: InsertBook :one
INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
VALUES ($1, $2, $3, $4, $5)
RETURNING id::text, owner_id::text, title, metadata_provenance, language_state,
          COALESCE(language_tag, '') AS language_tag, created_at, updated_at;

-- name: InsertBookMembership :exec
INSERT INTO book_membership(owner_id, book_id, state, activated_at)
VALUES ($1, $2, 'active', now());

-- name: UpdateBookMetadata :one
UPDATE books
SET title = $3, language_state = $4, language_tag = $5, updated_at = now()
WHERE owner_id = $1 AND id = $2
RETURNING id::text, owner_id::text, title, metadata_provenance, language_state,
          COALESCE(language_tag, '') AS language_tag, created_at, updated_at;

-- name: DeleteBookGoalsForLanguage :exec
DELETE FROM primary_goals WHERE owner_id = $1 AND book_id = $2 AND language <> $3;

-- name: DeleteBookGoals :exec
DELETE FROM primary_goals WHERE owner_id = $1 AND book_id = $2;

-- name: ActivateBookMembership :exec
INSERT INTO book_membership(owner_id, book_id, state, activated_at)
VALUES ($1, $2, 'active', now())
ON CONFLICT(owner_id, book_id) DO UPDATE SET
  state = 'active',
  activated_at = CASE WHEN book_membership.state = 'removed' THEN now() ELSE book_membership.activated_at END,
  removed_at = NULL;

-- name: RemoveBookMembership :exec
INSERT INTO book_membership(owner_id, book_id, state, removed_at)
VALUES ($1, $2, 'removed', now())
ON CONFLICT(owner_id, book_id) DO UPDATE SET
  state = 'removed',
  removed_at = CASE WHEN book_membership.state = 'removed' THEN book_membership.removed_at ELSE now() END;

-- name: GetBookByAlias :one
SELECT b.id::text, b.owner_id::text, b.title, b.metadata_provenance, b.language_state,
       COALESCE(b.language_tag, '') AS language_tag, b.created_at, b.updated_at
FROM books b
JOIN book_aliases a ON a.owner_id = b.owner_id AND a.book_id = b.id
WHERE a.owner_id = $1 AND a.namespace = $2 AND a.value = $3;

-- name: InsertBookAlias :one
INSERT INTO book_aliases(owner_id, book_id, alias_type, namespace, value)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(owner_id, namespace, value) WHERE connection_id IS NULL DO NOTHING
RETURNING id::text;

-- name: GetUnscopedAliasBookForUpdate :one
SELECT (COALESCE(book_id::text, ''))::text AS book_id
FROM book_aliases
WHERE owner_id = $1 AND namespace = $2 AND value = $3 AND connection_id IS NULL
FOR UPDATE;

-- name: GetSourceMaterialBookForUpdate :one
SELECT (COALESCE(book_id::text, ''))::text AS book_id
FROM source_materials
WHERE owner_id = $1 AND id = $2
FOR UPDATE;

-- name: GetSourceMaterialBookByIdentifierForUpdate :one
SELECT (COALESCE(book_id::text, ''))::text AS book_id
FROM source_materials
WHERE owner_id = $1 AND source_identifier = $2
FOR UPDATE;

-- name: SetSourceMaterialBook :exec
UPDATE source_materials SET book_id = $3 WHERE owner_id = $1 AND id = $2;

-- name: GetCatalogueAliasBookForUpdate :one
SELECT book_id::text
FROM book_aliases
WHERE owner_id = $1 AND connection_id = $2 AND namespace = $3 AND value = $4
FOR UPDATE;

-- name: GetBookMetadata :one
SELECT title, language_state, COALESCE(language_tag, '') AS language_tag
FROM books WHERE owner_id = $1 AND id = $2;

-- name: InsertCatalogueEntryAlias :exec
INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetBookByUnscopedAliasForUpdate :one
SELECT b.id::text, b.owner_id::text, b.title, b.metadata_provenance, b.language_state,
       COALESCE(b.language_tag, '') AS language_tag, b.created_at, b.updated_at
FROM books b
JOIN book_aliases a ON a.owner_id = b.owner_id AND a.book_id = b.id
WHERE a.owner_id = $1 AND a.namespace = $2 AND a.value = $3 AND a.connection_id IS NULL
FOR UPDATE OF b, a;
