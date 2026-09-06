DROP INDEX IF EXISTS book_aliases_catalogue_entry_identity;
DROP INDEX IF EXISTS book_aliases_strong_bibliographic_identity;

ALTER TABLE book_aliases
 DROP COLUMN IF EXISTS connection_id;
