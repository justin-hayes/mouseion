-- The embedded Classical catalog manifest is reconciled into these rows at
-- startup (ADR 0116). The Work, Edition and Reading-unit keys stay in the
-- existing definition_key columns: a Work row holds its Work key, an Edition
-- row its Edition key and a Reading unit row its permanent Reading-unit key.
--
-- Reconciliation needs to compare the running manifest with the previously
-- reconciled one, so the registry records the definition revision and the
-- content-bearing translation pin it last applied. A removed definition is a
-- withdrawn tombstone on its Reading-unit row: Work and Edition rows stay shared
-- by the remaining units, and the catalog derives their visibility from the
-- current manifest. Existing Books therefore never dangle.
ALTER TABLE public.works
    ADD COLUMN greek_title text NOT NULL DEFAULT '';

ALTER TABLE public.reading_units
    ADD COLUMN source_layer text NOT NULL DEFAULT '',
    ADD COLUMN source_format text NOT NULL DEFAULT '',
    ADD COLUMN constituent_book integer,
    ADD COLUMN constituent_books integer NOT NULL DEFAULT 1,
    ADD COLUMN word_count integer NOT NULL DEFAULT 0,
    ADD COLUMN revision integer NOT NULL DEFAULT 1,
    ADD COLUMN withdrawn boolean NOT NULL DEFAULT false,
    ADD COLUMN translation_layer text,
    ADD COLUMN translation_file_name text,
    ADD COLUMN translation_file_size bigint,
    ADD COLUMN translation_file_sha256 text,
    ADD COLUMN translation_citation_start text,
    ADD COLUMN translation_citation_end text;

ALTER TABLE ONLY public.reading_units
    ADD CONSTRAINT reading_units_constituent_book_check CHECK (
        constituent_book IS NULL
        OR (constituent_book >= 1 AND constituent_book <= constituent_books)
    ),
    ADD CONSTRAINT reading_units_constituent_books_check CHECK (constituent_books > 0),
    ADD CONSTRAINT reading_units_word_count_check CHECK (word_count >= 0),
    ADD CONSTRAINT reading_units_revision_check CHECK (revision > 0);

-- A translation pin is either absent or complete, so a partial pin can never
-- be mistaken for a recorded one during reconciliation.
ALTER TABLE ONLY public.reading_units
    ADD CONSTRAINT reading_units_translation_pin_check CHECK (
        (
            translation_layer IS NULL
            AND translation_file_name IS NULL
            AND translation_file_size IS NULL
            AND translation_file_sha256 IS NULL
            AND translation_citation_start IS NULL
            AND translation_citation_end IS NULL
        )
        OR (
            btrim(translation_layer) <> ''
            AND btrim(translation_file_name) <> ''
            AND translation_file_size > 0
            AND translation_file_sha256 ~ '^[0-9a-f]{64}$'
            AND translation_citation_start IS NOT NULL
            AND translation_citation_end IS NOT NULL
        )
    );
