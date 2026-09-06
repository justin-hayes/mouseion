ALTER TABLE book_aliases
 DROP CONSTRAINT book_aliases_owner_id_namespace_value_key,
 ADD CONSTRAINT book_aliases_connection_contract CHECK (
  (alias_type = 'catalog_entry' AND connection_id IS NOT NULL) OR
  (alias_type = 'strong_bibliographic' AND connection_id IS NULL)
 ) NOT VALID;
