ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_check1;
ALTER TABLE enrichment_cache DROP COLUMN gloss;
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_translation_required CHECK (translation <> ''),
 ADD CONSTRAINT enrichment_cache_sense_selection_array CHECK (jsonb_typeof(sense_selection) = 'array');
