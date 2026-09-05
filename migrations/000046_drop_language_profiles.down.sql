CREATE TABLE language_profiles (
	 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	 language text NOT NULL,
	 display_name text NOT NULL,
	 created_at timestamptz NOT NULL DEFAULT now(),
	 UNIQUE(owner_id, language)
);

ALTER TABLE language_profiles
ADD CONSTRAINT language_profiles_supported_language_fkey
FOREIGN KEY (language) REFERENCES supported_languages(language);
