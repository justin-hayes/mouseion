ALTER TABLE enrichment_cache
 ADD COLUMN sentence_translation_target text NOT NULL DEFAULT '';

COMMENT ON COLUMN enrichment_cache.sentence_translation_target IS 'Optional plain-text English word or phrase corresponding to the target in sentence_translation; provider markup is never stored';
