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