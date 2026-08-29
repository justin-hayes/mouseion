ALTER TABLE enrichment_cache
 ADD COLUMN target_language text NOT NULL DEFAULT 'en';

ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash, target_language);
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_target_language_nonempty CHECK (target_language <> '');

ALTER TABLE deck_preparation_runs
 ADD COLUMN execution_mode text NOT NULL DEFAULT 'batch',
 ADD COLUMN target_language text NOT NULL DEFAULT 'en';
ALTER TABLE deck_preparation_runs
 ADD CONSTRAINT deck_preparation_runs_execution_mode_check CHECK (execution_mode IN ('standard','batch')),
 ADD CONSTRAINT deck_preparation_runs_target_language_check CHECK (target_language <> '');

ALTER TABLE deck_preparation_manifest_items
 ADD COLUMN target_language text NOT NULL DEFAULT 'en';
ALTER TABLE deck_preparation_manifest_items
 ADD CONSTRAINT deck_preparation_manifest_items_target_language_check CHECK (target_language <> '');

CREATE FUNCTION reject_deck_preparation_run_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.execution_mode <> OLD.execution_mode OR NEW.target_language <> OLD.target_language THEN
  RAISE EXCEPTION 'prepared-deck run execution identity is immutable';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER deck_preparation_runs_identity_immutable
 BEFORE UPDATE ON deck_preparation_runs FOR EACH ROW
 EXECUTE FUNCTION reject_deck_preparation_run_identity_mutation();

COMMENT ON COLUMN enrichment_cache.target_language IS 'Explicit target language component of the immutable shared enrichment identity';
COMMENT ON COLUMN deck_preparation_runs.execution_mode IS 'Frozen prepared-deck executor mode: standard or batch';
COMMENT ON COLUMN deck_preparation_runs.target_language IS 'Frozen prepared-deck translation target language';
COMMENT ON COLUMN deck_preparation_manifest_items.target_language IS 'Frozen target language for this manifest item cache identity';
