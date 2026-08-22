-- OPDS connections are server configuration shared by every authenticated user.
-- Preserve every existing connection. Names which collide across former owners
-- receive a stable UUID suffix rather than silently discarding credentials or
-- choosing one user's configuration over another.
ALTER TABLE opds_connections DROP CONSTRAINT opds_connections_owner_id_name_key;

WITH duplicate_names AS (
    SELECT id
    FROM (
        SELECT id, count(*) OVER (PARTITION BY name) AS copies
        FROM opds_connections
    ) connections
    WHERE copies > 1
)
UPDATE opds_connections
SET name = name || ' [' || id::text || ']'
WHERE id IN (SELECT id FROM duplicate_names);

DROP INDEX opds_connections_owner_id_idx;
ALTER TABLE opds_connections DROP COLUMN owner_id;
ALTER TABLE opds_connections ADD CONSTRAINT opds_connections_name_key UNIQUE (name);
