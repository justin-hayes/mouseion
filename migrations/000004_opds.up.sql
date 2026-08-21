CREATE TABLE opds_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    url text NOT NULL,
    username text NOT NULL DEFAULT '',
    password_encrypted bytea NOT NULL DEFAULT ''::bytea,
    language text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, name)
);

CREATE INDEX opds_connections_owner_id_idx ON opds_connections(owner_id);

COMMENT ON COLUMN opds_connections.password_encrypted IS
    'AES-256-GCM ciphertext (nonce prefixed), encrypted by MOUSEION_SECRET';
