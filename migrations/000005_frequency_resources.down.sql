DROP INDEX IF EXISTS frequency_entries_lookup_idx;
DROP INDEX IF EXISTS frequency_datasets_one_active_per_language;
ALTER TABLE frequency_entries DROP COLUMN IF EXISTS frequency_class;
ALTER TABLE frequency_datasets
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS attribution,
    DROP COLUMN IF EXISTS license,
    DROP COLUMN IF EXISTS source_url;
