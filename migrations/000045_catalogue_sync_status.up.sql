CREATE TABLE catalogue_sync_status (
	owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	connection_id uuid NOT NULL REFERENCES opds_connections(id) ON DELETE CASCADE,
	state text NOT NULL CHECK (state IN ('syncing','synced','failed')),
	last_synced_at timestamptz,
	last_upserted_count integer NOT NULL DEFAULT 0,
	last_error text NOT NULL DEFAULT '',
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (owner_id, connection_id)
);

COMMENT ON COLUMN catalogue_sync_status.last_upserted_count IS
	'Books added or updated by the most recent successful sync; zero means the collection was already current.';
