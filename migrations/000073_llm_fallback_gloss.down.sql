ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_translation_required;
ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_sense_selection_array;
ALTER TABLE enrichment_cache ADD COLUMN gloss text NOT NULL DEFAULT '';
ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash, target_language);
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_check1 CHECK (translation <> '' OR gloss <> '');
ALTER TABLE enrichment_cache
 DROP COLUMN dictionary_provider_version,
 DROP COLUMN fallback_gloss,
 DROP COLUMN sense_selection;
