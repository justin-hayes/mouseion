-- Enforce the language identity after 000056 has populated every row.
ALTER TABLE primary_goals
  ALTER COLUMN language DROP DEFAULT,
  ADD CONSTRAINT primary_goals_language_nonempty CHECK (btrim(language) <> '');
