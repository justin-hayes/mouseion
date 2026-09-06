ALTER TABLE book_aliases
 DROP CONSTRAINT IF EXISTS book_aliases_connection_contract,
 ADD CONSTRAINT book_aliases_owner_id_namespace_value_key UNIQUE(owner_id, namespace, value);
