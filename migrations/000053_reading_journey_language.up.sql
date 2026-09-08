-- Structural half of the Journey rekey. The empty language is a temporary
-- legacy marker consumed by 000054_reading_journey_language_backfill.
ALTER TABLE reading_journeys
  ADD COLUMN language text NOT NULL DEFAULT '';

ALTER TABLE reading_journey_membership
  ADD COLUMN language text NOT NULL DEFAULT '',
  DROP CONSTRAINT reading_journey_membership_owner_id_fkey;

ALTER TABLE reading_journeys
  DROP CONSTRAINT reading_journeys_pkey,
  ADD PRIMARY KEY (owner_id, language);

ALTER TABLE reading_journey_membership
  DROP CONSTRAINT reading_journey_membership_pkey,
  ADD PRIMARY KEY (owner_id, language, book_id),
  ADD CONSTRAINT reading_journey_membership_journey_fkey
    FOREIGN KEY (owner_id, language)
    REFERENCES reading_journeys(owner_id, language) ON DELETE CASCADE;

DROP INDEX reading_journey_membership_order_idx;
CREATE INDEX reading_journey_membership_order_idx
  ON reading_journey_membership(owner_id, language, position, created_at, book_id);
