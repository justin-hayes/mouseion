DO $$
DECLARE
  entry     record;
  last_owner uuid := NULL;
  position  int := 0;
BEGIN
  FOR entry IN
    SELECT first_per_book.owner_id, first_per_book.book_id,
           first_per_book.created_at, first_per_book.campaign_id
    FROM (
      SELECT DISTINCT ON (c.owner_id, b.id)
             c.owner_id, b.id AS book_id, c.created_at, c.id AS campaign_id
      FROM learning_campaigns c
      JOIN source_materials s ON s.owner_id = c.owner_id AND s.id = c.source_material_id
      JOIN books b ON b.owner_id = s.owner_id AND b.id = s.book_id
      WHERE c.status = 'queued'
      ORDER BY c.owner_id, b.id, c.created_at, c.id
    ) first_per_book
    ORDER BY first_per_book.owner_id, first_per_book.created_at,
             first_per_book.campaign_id, first_per_book.book_id
  LOOP
    IF last_owner IS DISTINCT FROM entry.owner_id THEN
      INSERT INTO reading_journeys(owner_id) VALUES (entry.owner_id)
      ON CONFLICT (owner_id) DO NOTHING;
      last_owner := entry.owner_id;
      position := 0;
    END IF;
    position := position + 1;
    INSERT INTO reading_journey_membership(owner_id, book_id, position)
    VALUES (entry.owner_id, entry.book_id, position)
    ON CONFLICT (owner_id, book_id) DO NOTHING;
  END LOOP;
END
$$;
