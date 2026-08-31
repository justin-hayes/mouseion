-- 000037 My Books backfill (ADR 0035): links every existing source_materials row to a new Book.
-- Governance: additive backfill; idempotent and retry-safe (WHERE book_id IS NULL, SELECT FOR UPDATE on alias).
-- Advisory-lock style is per-source loop with alias conflict check; ambiguous alias collision raises EXCEPTION for investigation rather than merging.
-- Transaction: whole DO block runs in migration transaction; failure rolls back this file.
-- Rollback/forward-fix: down removes only backfill-created rows (metadata_provenance='source_materials_backfill'); forward-fix preferred for partial failures.
DO $$
DECLARE
 source_record record;
 existing_book uuid;
 new_book uuid;
BEGIN
 FOR source_record IN
  SELECT id, owner_id, title, language, source_identifier
  FROM source_materials
  WHERE book_id IS NULL
  ORDER BY owner_id, id
 LOOP
  -- A pre-existing alias would make it impossible to prove that the
  -- backfill-created Book is the sole owner of this legacy identity.
  SELECT a.book_id INTO existing_book
  FROM book_aliases a
  WHERE a.owner_id = source_record.owner_id
    AND a.namespace = 'source_identifier'
    AND a.value = source_record.source_identifier
  FOR UPDATE;
  IF FOUND THEN
   RAISE EXCEPTION 'my books backfill alias collision for owner % source % (book %)',
    source_record.owner_id, source_record.source_identifier, existing_book;
  END IF;

  INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
  VALUES (source_record.owner_id, source_record.title, 'source_materials_backfill', 'chosen', source_record.language)
  RETURNING id INTO new_book;

  INSERT INTO book_membership(owner_id, book_id, state, activated_at)
  VALUES (source_record.owner_id, new_book, 'active', now());

  INSERT INTO book_aliases(owner_id, book_id, alias_type, namespace, value)
  VALUES (source_record.owner_id, new_book, 'catalog_entry', 'source_identifier', source_record.source_identifier);

  UPDATE source_materials
  SET book_id = new_book
  WHERE owner_id = source_record.owner_id AND id = source_record.id AND book_id IS NULL;
 END LOOP;
END
$$;
