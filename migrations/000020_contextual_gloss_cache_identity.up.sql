ALTER TABLE public.enrichment_cache
    ADD COLUMN meaning_evidence_hash text NOT NULL DEFAULT '';

ALTER TABLE public.enrichment_cache
    DROP CONSTRAINT enrichment_cache_pkey,
    ADD CONSTRAINT enrichment_cache_meaning_evidence_hash_format
        CHECK (meaning_evidence_hash = '' OR meaning_evidence_hash ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT enrichment_cache_pkey PRIMARY KEY
        (language, canonical_lemma, upos, provider, provider_version,
         sentence_hash, target_language, dictionary_provider_version,
         meaning_evidence_hash);

COMMENT ON COLUMN public.enrichment_cache.meaning_evidence_hash IS
    'Lowercase SHA-256 identity of the exact ordered lexical evidence used for a contextual Gloss; empty for historical non-contextual results';

ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN meaning_evidence_hash text;

ALTER TABLE public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_items_meaning_evidence_hash_format
        CHECK (meaning_evidence_hash IS NULL OR meaning_evidence_hash ~ '^[0-9a-f]{64}$');
