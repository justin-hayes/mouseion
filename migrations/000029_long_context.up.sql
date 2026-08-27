ALTER TABLE enrichment_cache
 ADD COLUMN context_sentence text NOT NULL DEFAULT '';

COMMENT ON COLUMN enrichment_cache.context_sentence IS 'Validated shorter exact substring of the approved source sentence; full sentence when no safe shortening was returned';
