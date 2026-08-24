DROP INDEX opds_connections_owner_name_key;
DROP INDEX opds_connections_owner_id_idx;

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

ALTER TABLE opds_connections DROP COLUMN owner_id;
ALTER TABLE opds_connections ADD CONSTRAINT opds_connections_name_key UNIQUE (name);
