CREATE TABLE enrichment_cache (
 language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 provider text NOT NULL, provider_version text NOT NULL,
 translation text NOT NULL DEFAULT '', gloss text NOT NULL DEFAULT '',
 cached_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version),
 CHECK (language <> '' AND canonical_lemma <> '' AND upos <> '' AND provider <> '' AND provider_version <> ''),
 CHECK (translation <> '' OR gloss <> '')
);
COMMENT ON TABLE enrichment_cache IS 'Shared, language-scoped, immutable external enrichment results; contains no owner or source metadata';
CREATE FUNCTION reject_enrichment_cache_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'enrichment cache rows are immutable'; END; $$;
CREATE TRIGGER enrichment_cache_immutable BEFORE UPDATE ON enrichment_cache FOR EACH ROW EXECUTE FUNCTION reject_enrichment_cache_mutation();
