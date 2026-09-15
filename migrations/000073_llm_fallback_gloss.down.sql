ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash, target_language);
ALTER TABLE enrichment_cache
 DROP COLUMN dictionary_provider_version,
 DROP COLUMN fallback_gloss,
 DROP COLUMN sense_selection;
