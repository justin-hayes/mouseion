ALTER TABLE public.enrichment_cache
    ADD COLUMN sentence_translation_targets text[] NOT NULL DEFAULT ARRAY[]::text[];

COMMENT ON COLUMN public.enrichment_cache.sentence_translation_targets IS
    'Optional ordered plain-text excerpts proposed for English target alignment; markup is never stored';
