ALTER TABLE frequency_datasets
    ADD COLUMN source_url text NOT NULL DEFAULT '',
    ADD COLUMN license text NOT NULL DEFAULT '',
    ADD COLUMN attribution text NOT NULL DEFAULT '',
    ADD COLUMN active boolean NOT NULL DEFAULT false;

ALTER TABLE frequency_entries
    ADD COLUMN frequency_class smallint NOT NULL DEFAULT 0
        CHECK (frequency_class BETWEEN 0 AND 6);

CREATE UNIQUE INDEX frequency_datasets_one_active_per_language
    ON frequency_datasets(language) WHERE active;
CREATE INDEX frequency_entries_lookup_idx
    ON frequency_entries(language, canonical_lemma, upos);
