DO $$
DECLARE
  goal record;
BEGIN
  FOR goal IN
    SELECT c.owner_id, b.id AS book_id
    FROM learning_campaigns c
    JOIN source_materials s ON s.owner_id = c.owner_id AND s.id = c.source_material_id
    JOIN books b ON b.owner_id = s.owner_id AND b.id = s.book_id
    WHERE c.status = 'active'
    ORDER BY c.owner_id
  LOOP
    INSERT INTO primary_goals(owner_id, book_id)
    VALUES (goal.owner_id, goal.book_id)
    ON CONFLICT (owner_id) DO NOTHING;
  END LOOP;
END
$$;
