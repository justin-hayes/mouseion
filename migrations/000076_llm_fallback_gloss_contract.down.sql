ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_translation_required;
ALTER TABLE enrichment_cache DROP CONSTRAINT enrichment_cache_sense_selection_array;
ALTER TABLE enrichment_cache ADD COLUMN gloss text NOT NULL DEFAULT '';
ALTER TABLE enrichment_cache
 ADD CONSTRAINT enrichment_cache_check1 CHECK (translation <> '' OR gloss <> '');
