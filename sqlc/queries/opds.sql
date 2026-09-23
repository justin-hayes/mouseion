-- OPDS connection queries. Credential decryption stays in Go; these queries
-- own the row iteration and RETURNING composition.

-- name: CreateOpdsConnection :one
INSERT INTO opds_connections(owner_id, name, url, username, password_encrypted, language)
VALUES ($1, $2, $3, $4, $5, '')
RETURNING id::text, owner_id::text, name, url, username, password_encrypted, language, created_at, updated_at;

-- name: GetOpdsConnection :one
SELECT id::text, owner_id::text, name, url, username, password_encrypted, language, created_at, updated_at
FROM opds_connections WHERE owner_id = $1 AND id = $2;

-- name: ListOpdsConnections :many
SELECT id::text, owner_id::text, name, url, username, password_encrypted, language, created_at, updated_at
FROM opds_connections WHERE owner_id = $1 ORDER BY name, id;

-- name: ListAllOpdsConnectionIDs :many
SELECT id::text, owner_id::text FROM opds_connections WHERE owner_id IS NOT NULL ORDER BY owner_id, id;

-- name: OpdsConnectionExists :one
SELECT EXISTS(SELECT 1 FROM opds_connections WHERE owner_id = $1 AND id = $2);

-- name: UpdateOpdsConnection :one
UPDATE opds_connections SET name = $3, url = $4, username = $5, password_encrypted = $6, updated_at = now()
WHERE owner_id = $1 AND id = $2
RETURNING id::text, owner_id::text, name, url, username, password_encrypted, language, created_at, updated_at;

-- name: DeleteOpdsConnection :execrows
DELETE FROM opds_connections WHERE owner_id = $1 AND id = $2;

-- name: DeleteBookCoverCandidatesForConnection :exec
DELETE FROM book_cover_candidates
WHERE owner_id = sqlc.arg('owner_id') AND connection_id = sqlc.arg('connection_id');

-- name: ResolveBookCoverAfterConnectionDeletion :exec
UPDATE book_covers AS cover
SET state = 'unavailable',
    media_type = NULL,
    width = NULL,
    height = NULL,
    content_hash = NULL,
    bytes = NULL,
    fetched_at = NULL,
    failure_reason = 'cover retrieval source was deleted',
    updated_at = now()
WHERE cover.owner_id = sqlc.arg('owner_id')
  AND cover.state = 'pending'
  AND cover.selected_connection_id = sqlc.arg('connection_id')
  AND NOT EXISTS (
      SELECT 1
      FROM book_cover_candidates AS candidate
      WHERE candidate.owner_id = cover.owner_id
        AND candidate.book_id = cover.book_id
        AND candidate.advertised_at = cover.advertised_at
        AND candidate.state = 'pending'
  );
