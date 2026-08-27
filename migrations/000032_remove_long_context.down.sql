ALTER TABLE enrichment_cache
 ADD COLUMN IF NOT EXISTS context_sentence text NOT NULL DEFAULT '';

COMMENT ON COLUMN enrichment_cache.context_sentence IS 'Deprecated legacy column; no application code reads or writes it';
