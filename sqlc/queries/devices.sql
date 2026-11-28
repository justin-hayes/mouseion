-- Devices and their credentials (ADR 0143, spec #2868). Every query is scoped
-- to the owner; the key hash never leaves the database in a list.

-- name: InsertDevice :one
-- Stores a new Device. A taken username violates devices_username_key.
INSERT INTO devices (owner_id, name, username, key_hash)
VALUES (sqlc.arg('owner'), sqlc.arg('name'), sqlc.arg('username'), sqlc.arg('key_hash'))
RETURNING id::text AS id, name, username, created_at, last_seen_at, revoked_at;

-- name: ListDevices :many
-- The owner's Devices, revoked ones included, oldest first.
SELECT id::text AS id, name, username, created_at, last_seen_at, revoked_at
  FROM devices
 WHERE owner_id = sqlc.arg('owner')
 ORDER BY created_at, id;
