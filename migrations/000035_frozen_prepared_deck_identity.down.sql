DROP TRIGGER IF EXISTS deck_preparation_runs_identity_immutable ON deck_preparation_runs;
DROP FUNCTION IF EXISTS reject_deck_preparation_run_identity_mutation();

ALTER TABLE deck_preparation_manifest_items
 DROP CONSTRAINT IF EXISTS deck_preparation_manifest_items_target_language_check,
 DROP COLUMN IF EXISTS target_language;
ALTER TABLE deck_preparation_runs
 DROP CONSTRAINT IF EXISTS deck_preparation_runs_execution_mode_check,
 DROP CONSTRAINT IF EXISTS deck_preparation_runs_target_language_check,
 DROP COLUMN IF EXISTS execution_mode,
 DROP COLUMN IF EXISTS target_language;

ALTER TABLE enrichment_cache DROP CONSTRAINT IF EXISTS enrichment_cache_pkey;
ALTER TABLE enrichment_cache
 ADD PRIMARY KEY(language, canonical_lemma, upos, provider, provider_version, sentence_hash);
ALTER TABLE enrichment_cache DROP CONSTRAINT IF EXISTS enrichment_cache_target_language_nonempty;
ALTER TABLE enrichment_cache DROP COLUMN IF EXISTS target_language;
