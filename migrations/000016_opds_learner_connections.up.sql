-- Restore learner ownership without discarding shared connections created while
-- OPDS configuration was server-wide. Migration 000010 erased their original
-- owners, so assign them deterministically to the oldest administrator. That
-- account becomes an ordinary learner in the follow-up account migration.
ALTER TABLE opds_connections
    ADD COLUMN owner_id uuid REFERENCES users(id) ON DELETE CASCADE;

UPDATE opds_connections
SET owner_id = (
    SELECT id
    FROM users
    WHERE is_admin
    ORDER BY created_at, id
    LIMIT 1
);

ALTER TABLE opds_connections DROP CONSTRAINT opds_connections_name_key;

CREATE INDEX opds_connections_owner_id_idx ON opds_connections(owner_id);
CREATE UNIQUE INDEX opds_connections_owner_name_key
    ON opds_connections(owner_id, name)
    WHERE owner_id IS NOT NULL;
