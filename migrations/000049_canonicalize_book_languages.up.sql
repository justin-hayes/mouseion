-- Data-only convergence for ADR 0046. The canonical form is the lowercased
-- base subtag after normalizing underscores to hyphens. This migration is
-- deterministic and safe to rerun.

-- Keep the oldest known-vocabulary row when regional spellings converge to
-- the same owner/language/lemma identity. The rows contain no downstream
-- references, so deleting the later duplicate preserves one fact safely.
WITH ranked AS (
	SELECT id,
		ROW_NUMBER() OVER (
			PARTITION BY owner_id,
				lower(split_part(replace(trim(language), '_', '-'), '-', 1)),
				canonical_lemma, upos
			ORDER BY created_at, id
		) AS row_number
	FROM known_vocabulary
)
DELETE FROM known_vocabulary AS known
USING ranked
WHERE known.id = ranked.id
	AND ranked.row_number > 1;

-- Keep the most recently entered display name for converging global reference
-- rows, matching the original supported-language import policy.
WITH ranked AS (
	SELECT language,
		ROW_NUMBER() OVER (
			PARTITION BY lower(split_part(replace(trim(language), '_', '-'), '-', 1))
			ORDER BY created_at DESC, language DESC
		) AS row_number
	FROM supported_languages
)
DELETE FROM supported_languages AS supported
USING ranked
WHERE supported.language = ranked.language
	AND ranked.row_number > 1;

UPDATE books
SET language_tag = lower(split_part(replace(trim(language_tag), '_', '-'), '-', 1))
WHERE language_state = 'chosen'
	AND trim(COALESCE(language_tag, '')) <> '';

UPDATE known_vocabulary
SET language = lower(split_part(replace(trim(language), '_', '-'), '-', 1));

UPDATE supported_languages
SET language = lower(split_part(replace(trim(language), '_', '-'), '-', 1));
