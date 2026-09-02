DROP TABLE IF EXISTS book_current_analyses;
ALTER TABLE source_materials
  DROP CONSTRAINT IF EXISTS source_materials_owner_id_id_book_id_key;
