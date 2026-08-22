ALTER TABLE opds_connections DROP CONSTRAINT opds_connections_name_key;
ALTER TABLE opds_connections ADD COLUMN owner_id uuid REFERENCES users(id) ON DELETE CASCADE;

-- Ownership cannot be reconstructed after migration. For rollback, assign
-- shared connections to the oldest administrator (or oldest user as a fallback).
UPDATE opds_connections
SET owner_id = COALESCE(
    (SELECT id FROM users WHERE is_admin ORDER BY created_at, id LIMIT 1),
    (SELECT id FROM users ORDER BY created_at, id LIMIT 1)
);

ALTER TABLE opds_connections ALTER COLUMN owner_id SET NOT NULL;
ALTER TABLE opds_connections ADD CONSTRAINT opds_connections_owner_id_name_key UNIQUE (owner_id, name);
CREATE INDEX opds_connections_owner_id_idx ON opds_connections(owner_id);
