ALTER TABLE book_aliases
 ADD COLUMN connection_id uuid,
 ADD CONSTRAINT book_aliases_connection_fkey
  FOREIGN KEY (connection_id) REFERENCES opds_connections(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX book_aliases_catalogue_entry_identity
 ON book_aliases(owner_id, connection_id, namespace, value)
 WHERE connection_id IS NOT NULL;

CREATE UNIQUE INDEX book_aliases_strong_bibliographic_identity
 ON book_aliases(owner_id, namespace, value)
 WHERE connection_id IS NULL;
