-- Data-only backfill of retained Book covers into catalog_entry_covers (ADR
-- 0130, ADR 0038). Structural changes are in 000064 (expand) and 000066
-- (contract); this file moves rows and changes no schema.
--
-- Owner: whoever runs the deploy; it runs once through the normal migration
-- Up. Idempotent and retry safe: ON CONFLICT DO NOTHING, so a re-run copies
-- nothing twice and never overwrites an entry row. Locking: one INSERT ... SELECT
-- that reads book_covers without blocking readers and locks only the new table.
-- No image is fetched: available images keep their bytes; Books still waiting
-- (pending, or a selection with no settled state) become pending entry rows and
-- are retrieved by the next Catalog sync. Recovery: forward fix; the source
-- book_covers rows are untouched until 000066, so this can be re-run.
--
-- Completion signal before 000066 (per selection, so pre-existing entry rows
-- cannot mask a missing copy): this returns 0 when every book_covers row with a
-- selection has its entry row. 000066 refuses to run unless it is 0.
--   SELECT count(*) FROM public.book_covers bc
--   WHERE bc.selected_connection_id IS NOT NULL
--     AND btrim(COALESCE(bc.selected_source_identifier, '')) <> ''
--     AND NOT EXISTS (SELECT 1 FROM public.catalog_entry_covers c
--       WHERE c.owner_id = bc.owner_id AND c.connection_id = bc.selected_connection_id
--         AND c.entry_identifier = bc.selected_source_identifier);
-- Two Books never share an entry (one entry links to at most one Book), but if
-- they did the best row (available, then most recently fetched) wins.
INSERT INTO public.catalog_entry_covers (
    owner_id, connection_id, entry_identifier, state, advertised_at,
    media_type, width, height, content_hash, bytes, fetched_at, failure_reason, updated_at)
SELECT DISTINCT ON (bc.owner_id, bc.selected_connection_id, bc.selected_source_identifier)
    bc.owner_id,
    bc.selected_connection_id,
    bc.selected_source_identifier,
    CASE bc.state WHEN 'available' THEN 'available' WHEN 'unavailable' THEN 'unavailable' ELSE 'pending' END,
    COALESCE(bc.advertised_at, bc.updated_at),
    CASE WHEN bc.state = 'available' THEN bc.media_type END,
    CASE WHEN bc.state = 'available' THEN bc.width END,
    CASE WHEN bc.state = 'available' THEN bc.height END,
    CASE WHEN bc.state = 'available' THEN bc.content_hash END,
    CASE WHEN bc.state = 'available' THEN bc.bytes END,
    bc.fetched_at,
    bc.failure_reason,
    bc.updated_at
FROM public.book_covers bc
WHERE bc.selected_connection_id IS NOT NULL
  AND bc.selected_source_identifier IS NOT NULL
  AND btrim(bc.selected_source_identifier) <> ''
ORDER BY bc.owner_id, bc.selected_connection_id, bc.selected_source_identifier,
    (bc.state = 'available') DESC, bc.fetched_at DESC NULLS LAST, bc.book_id
ON CONFLICT (owner_id, connection_id, entry_identifier) DO NOTHING;
