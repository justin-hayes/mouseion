-- The split is intentionally not reconstructed on rollback. Remove only the
-- language-partitioned data so the structural down migration can restore the
-- legacy shape without inventing a mixed-language order.
DELETE FROM reading_journey_membership WHERE language <> '';
DELETE FROM reading_journeys WHERE language <> '';

ALTER TABLE reading_journey_membership
  DROP CONSTRAINT reading_journey_membership_language_nonempty,
  ALTER COLUMN language SET DEFAULT '';
ALTER TABLE reading_journeys
  DROP CONSTRAINT reading_journeys_language_nonempty,
  ALTER COLUMN language SET DEFAULT '';
