-- Prefactor for idiom cards (ADR 0114, ticket #2360). A frozen deck item and
-- the contextual provider cache identity gain an item kind. Every row that
-- exists today is a lemma item, which is also the column default, so no
-- backfill is needed and no existing behaviour changes. This migration is
-- purely additive; it adds no idiom behaviour and no vocabulary state.

ALTER TABLE public.deck_preparation_manifest_items
    ADD COLUMN item_kind text NOT NULL DEFAULT 'lemma';

ALTER TABLE public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_items_item_kind_check
        CHECK (item_kind IN ('lemma', 'idiom'));

COMMENT ON COLUMN public.deck_preparation_manifest_items.item_kind IS
    'Frozen card kind: lemma for every vocabulary item, idiom for a dictionary-anchored multi-word idiom item';

ALTER TABLE public.enrichment_cache
    ADD COLUMN item_kind text NOT NULL DEFAULT 'lemma';

-- The kind is part of the contextual provider cache identity, so a lemma
-- result can never be served for an idiom with the same text, or vice versa.
ALTER TABLE public.enrichment_cache
    DROP CONSTRAINT enrichment_cache_pkey,
    ADD CONSTRAINT enrichment_cache_item_kind_check
        CHECK (item_kind IN ('lemma', 'idiom')),
    ADD CONSTRAINT enrichment_cache_pkey PRIMARY KEY
        (language, canonical_lemma, upos, item_kind, provider, provider_version,
         sentence_hash, target_language, dictionary_provider_version,
         meaning_evidence_hash);

COMMENT ON COLUMN public.enrichment_cache.item_kind IS
    'Card kind this contextual result belongs to; part of the cache identity';
