CREATE TABLE supported_languages (
 language text PRIMARY KEY,
 display_name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);

-- Preserve every configured language while separating server configuration
-- from learner-owned selections. Prefer the most recently entered name.
INSERT INTO supported_languages(language, display_name, created_at)
SELECT DISTINCT ON (language) language, display_name, created_at
FROM language_profiles
ORDER BY language, created_at DESC;

-- Administrator profiles represented the old server-wide configuration, not
-- a personal study choice. Learner profiles retain their owner scoping.
DELETE FROM language_profiles
WHERE owner_id IN (SELECT id FROM users WHERE is_admin);

ALTER TABLE language_profiles
 ADD CONSTRAINT language_profiles_supported_language_fkey
 FOREIGN KEY(language) REFERENCES supported_languages(language);
