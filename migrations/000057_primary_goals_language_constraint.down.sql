ALTER TABLE primary_goals
  DROP CONSTRAINT primary_goals_language_nonempty;

ALTER TABLE primary_goals
  ALTER COLUMN language SET DEFAULT '';
