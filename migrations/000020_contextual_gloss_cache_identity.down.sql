ALTER TABLE public.enrichment_cache
    DROP CONSTRAINT enrichment_cache_pkey,
    DROP CONSTRAINT enrichment_cache_meaning_evidence_hash_format,
    DROP COLUMN meaning_evidence_hash;

ALTER TABLE public.enrichment_cache
    ADD CONSTRAINT enrichment_cache_pkey PRIMARY KEY
        (language, canonical_lemma, upos, provider, provider_version,
         sentence_hash, target_language, dictionary_provider_version);

ALTER TABLE public.deck_preparation_manifest_items
    DROP CONSTRAINT deck_preparation_manifest_items_meaning_evidence_hash_format,
    DROP COLUMN meaning_evidence_hash;
