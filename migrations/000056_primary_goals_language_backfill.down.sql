-- The structural predecessor uses the empty language marker. Remove the
-- language-partitioned rows rather than inventing a merge policy for rollback.
DELETE FROM primary_goals WHERE language <> '';

ALTER TABLE primary_goals
  ALTER COLUMN language SET DEFAULT '';
