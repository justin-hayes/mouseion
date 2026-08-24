ALTER TABLE enrichment_cache
 ADD COLUMN sentence_hash text NOT NULL DEFAULT '',
 ADD COLUMN sentence_translation text NOT NULL DEFAULT '';

ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash);
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_sentence_hash_format
 CHECK (sentence_hash = '' OR sentence_hash ~ '^[0-9a-f]{64}$');

COMMENT ON COLUMN enrichment_cache.sentence_hash IS 'Lowercase hex SHA-256 of conservatively normalized approved sentence text; empty for lemma-only legacy entries';
COMMENT ON COLUMN enrichment_cache.sentence_translation IS 'Natural translation of the complete approved example sentence; empty when unavailable';
