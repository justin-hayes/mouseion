-- The legacy schema can represent only one Goal per owner. Do not invent a
-- merge policy when rolling back a language-partitioned set of Goals. This
-- destructive rollback is for development/recovery only; restore a database
-- backup when preserving split Goals matters.
DELETE FROM primary_goals;

ALTER TABLE primary_goals
  DROP CONSTRAINT primary_goals_pkey,
  ADD PRIMARY KEY (owner_id);

ALTER TABLE primary_goals
  DROP COLUMN language;
