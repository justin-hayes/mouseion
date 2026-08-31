CREATE TEMP TABLE my_books_backfill_targets ON COMMIT DROP AS
SELECT DISTINCT b.owner_id, b.id AS book_id, a.id AS alias_id
FROM books b
JOIN book_aliases a ON a.owner_id = b.owner_id AND a.book_id = b.id
JOIN source_materials s ON s.owner_id = b.owner_id
 AND s.book_id = b.id
 AND s.source_identifier = a.value
WHERE b.metadata_provenance = 'source_materials_backfill'
 AND a.alias_type = 'catalog_entry'
 AND a.namespace = 'source_identifier';

UPDATE source_materials s
SET book_id = NULL
FROM my_books_backfill_targets t
WHERE s.owner_id = t.owner_id AND s.book_id = t.book_id;

DELETE FROM book_aliases a
USING my_books_backfill_targets t
WHERE a.id = t.alias_id;

DELETE FROM book_membership m
USING my_books_backfill_targets t
WHERE m.owner_id = t.owner_id AND m.book_id = t.book_id;

DELETE FROM books b
USING my_books_backfill_targets t
WHERE b.owner_id = t.owner_id AND b.id = t.book_id;
