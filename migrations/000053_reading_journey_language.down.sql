DROP INDEX reading_journey_membership_order_idx;

ALTER TABLE reading_journey_membership
  DROP CONSTRAINT reading_journey_membership_journey_fkey,
  DROP CONSTRAINT reading_journey_membership_pkey,
  DROP COLUMN language,
  ADD PRIMARY KEY (owner_id, book_id);

ALTER TABLE reading_journeys
  DROP CONSTRAINT reading_journeys_pkey,
  DROP COLUMN language,
  ADD PRIMARY KEY (owner_id);

ALTER TABLE reading_journey_membership
  ADD CONSTRAINT reading_journey_membership_owner_id_fkey
    FOREIGN KEY (owner_id)
    REFERENCES reading_journeys(owner_id) ON DELETE CASCADE;

CREATE INDEX reading_journey_membership_order_idx
  ON reading_journey_membership(owner_id, position, created_at, book_id);
