-- Structural half of the Primary Goal rekey. The empty language marker is
-- filled by 000056_primary_goals_language_backfill.
ALTER TABLE primary_goals
  ADD COLUMN language text NOT NULL DEFAULT '';

ALTER TABLE primary_goals
  DROP CONSTRAINT primary_goals_pkey,
  ADD PRIMARY KEY (owner_id, language);
