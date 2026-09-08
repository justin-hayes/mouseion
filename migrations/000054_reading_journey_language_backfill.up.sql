-- Data-only, idempotent backfill for ADR 0051. The whole split is one
-- statement transaction: a retry either sees the completed split or reruns the
-- same ON CONFLICT-safe inserts. Unknown-language Books are intentionally not
-- carried into a language Journey.
-- Ownership: application startup runs this migration through golang-migrate.
-- Recovery: a failed run is rolled back atomically and is safe to retry.
-- Rollback: the paired down migration removes split rows without inventing a
-- mixed-language legacy order.
DO $$
BEGIN
  INSERT INTO reading_journeys(owner_id, language, revision, updated_at)
  SELECT DISTINCT j.owner_id, b.language_tag, j.revision, j.updated_at
  FROM reading_journeys j
  JOIN reading_journey_membership m
    ON m.owner_id = j.owner_id AND m.language = ''
  JOIN books b
    ON b.owner_id = m.owner_id AND b.id = m.book_id
  WHERE j.language = ''
    AND b.language_state = 'chosen'
    AND b.language_tag IS NOT NULL
    AND b.language_tag <> ''
  ON CONFLICT (owner_id, language) DO NOTHING;

  INSERT INTO reading_journey_membership(owner_id, language, book_id, position, created_at)
  SELECT m.owner_id, b.language_tag, m.book_id,
         row_number() OVER (
           PARTITION BY m.owner_id, b.language_tag
           ORDER BY m.position, m.created_at, m.book_id
         ),
         m.created_at
  FROM reading_journeys j
  JOIN reading_journey_membership m
    ON m.owner_id = j.owner_id AND m.language = ''
  JOIN books b
    ON b.owner_id = m.owner_id AND b.id = m.book_id
  WHERE j.language = ''
    AND b.language_state = 'chosen'
    AND b.language_tag IS NOT NULL
    AND b.language_tag <> ''
  ON CONFLICT (owner_id, language, book_id) DO NOTHING;

  DELETE FROM reading_journey_membership WHERE language = '';
  DELETE FROM reading_journeys WHERE language = '';

  ALTER TABLE reading_journeys ALTER COLUMN language DROP DEFAULT;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'reading_journeys'::regclass
      AND conname = 'reading_journeys_language_nonempty'
  ) THEN
    ALTER TABLE reading_journeys
      ADD CONSTRAINT reading_journeys_language_nonempty CHECK (btrim(language) <> '');
  END IF;
  ALTER TABLE reading_journey_membership ALTER COLUMN language DROP DEFAULT;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'reading_journey_membership'::regclass
      AND conname = 'reading_journey_membership_language_nonempty'
  ) THEN
    ALTER TABLE reading_journey_membership
      ADD CONSTRAINT reading_journey_membership_language_nonempty CHECK (btrim(language) <> '');
  END IF;
END
$$;
