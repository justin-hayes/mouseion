-- Reverse 000046_frozen_deck_item_kind. The pre-kind schema cannot distinguish
-- an idiom cache row from a lemma row with the same text, so the restored
-- primary key would reject the pair. The cache is derived data; discard the
-- idiom rows the downgrade cannot represent and keep every lemma row.
--
-- Frozen manifest rows are left in place. An idiom manifest becomes unreadable
-- after the downgrade because its v14 digest still includes the kind the
-- column no longer carries, which fails the existing identity check instead of
-- silently reading an idiom as a lemma. As with every down migration here, this
-- is not assumed safe for destructive production rollback.

DELETE FROM public.enrichment_cache WHERE item_kind = 'idiom';

ALTER TABLE public.enrichment_cache
    DROP CONSTRAINT enrichment_cache_pkey,
    DROP CONSTRAINT enrichment_cache_item_kind_check,
    DROP COLUMN item_kind;

ALTER TABLE public.enrichment_cache
    ADD CONSTRAINT enrichment_cache_pkey PRIMARY KEY
        (language, canonical_lemma, upos, provider, provider_version,
         sentence_hash, target_language, dictionary_provider_version,
         meaning_evidence_hash);

ALTER TABLE public.deck_preparation_manifest_items
    DROP CONSTRAINT deck_preparation_manifest_items_item_kind_check,
    DROP COLUMN item_kind;
