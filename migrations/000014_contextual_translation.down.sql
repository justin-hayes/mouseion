DELETE FROM enrichment_cache WHERE sentence_hash <> '';

ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_sentence_hash_format;
ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version);

ALTER TABLE enrichment_cache
 DROP COLUMN sentence_translation,
 DROP COLUMN sentence_hash;
