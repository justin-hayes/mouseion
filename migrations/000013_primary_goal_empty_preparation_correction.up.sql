-- Migrations 000008 and 000011 preserve the activity evidence needed to
-- classify empty legacy preparations. There is no safe corrective action left
-- for this migration to perform: any bound empty preparation may be the
-- genuinely active study adopted by 000008. Do not detach it speculatively.
--
-- A backup from before migration 000008 remains the only recovery posture for
-- an installation that already ran the old migration text. This migration is
-- intentionally a no-op for new installs and is safe to rerun. Existing deck
-- artifacts and generated-vocabulary provenance are left untouched.
SELECT 1
WHERE false;
