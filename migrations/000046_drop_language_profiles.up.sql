-- This destructive cleanup removes obsolete learner preferences. Production
-- recovery requires a database backup taken before this migration; the down
-- migration recreates the table shape but cannot restore deleted rows.
ALTER TABLE language_profiles
DROP CONSTRAINT language_profiles_supported_language_fkey;

DROP TABLE language_profiles;
