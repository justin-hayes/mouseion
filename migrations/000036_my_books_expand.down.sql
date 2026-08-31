ALTER TABLE source_materials
 DROP CONSTRAINT IF EXISTS source_materials_book_fkey,
 DROP COLUMN IF EXISTS book_id;

DROP INDEX IF EXISTS source_materials_owner_book_idx;
DROP TABLE IF EXISTS book_aliases;
DROP TABLE IF EXISTS book_membership;
DROP TABLE IF EXISTS books;
