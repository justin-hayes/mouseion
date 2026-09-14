ALTER TABLE enrichment_cache
 ADD COLUMN dictionary_provider_version text NOT NULL DEFAULT '',
 ADD COLUMN fallback_gloss text NOT NULL DEFAULT '',
 ADD COLUMN sense_selection jsonb NOT NULL DEFAULT '[]'::jsonb;

-- Legacy gloss-only rows cannot satisfy the translation-required cache
-- contract and are not usable by the enrichment pipeline.
DELETE FROM enrichment_cache WHERE translation = '';

ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash, target_language, dictionary_provider_version);

ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_check1;
ALTER TABLE enrichment_cache DROP COLUMN gloss;
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_translation_required CHECK (translation <> ''),
 ADD CONSTRAINT enrichment_cache_sense_selection_array CHECK (jsonb_typeof(sense_selection) = 'array');

COMMENT ON COLUMN enrichment_cache.dictionary_provider_version IS 'Dictionary index identity used to produce the frozen card meaning candidates';
COMMENT ON COLUMN enrichment_cache.fallback_gloss IS 'Consent-gated external gloss used only when the frozen dictionary meaning is unavailable';
COMMENT ON COLUMN enrichment_cache.sense_selection IS 'Reserved durable ordered dictionary sense indices for the consented selection phase';
