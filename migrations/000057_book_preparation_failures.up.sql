-- One durable failure record per Book preparation (ADR 0121). Classical
-- preparation and EPUB acquisition share the key, stage and code vocabulary;
-- they differ only in provenance. A Book is only ever one kind, so the key
-- never collides.
--
-- Classical provenance is the Reading unit key and definition revision. EPUB
-- provenance is the owner-scoped Catalog connection and entry identifier. Both
-- sets are nullable, and the provenance check requires exactly one kind. An
-- entry-stage EPUB failure may have neither identity when acquisition could not
-- resolve an entry at all; every other failure names its catalog entry.
--
-- A Classical failure is compared with the current Reading unit revision, so a
-- newer definition revision clears it without rewriting the row. An EPUB row is
-- deleted when acquisition succeeds. Only stable stages and ASCII codes are
-- stored; source text and Greek are never written here.
CREATE TABLE public.book_preparation_failures (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    reading_unit_key text,
    definition_revision integer,
    connection_id uuid,
    entry_identifier text,
    stage text NOT NULL,
    code text NOT NULL,
    failed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT book_preparation_failures_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT book_preparation_failures_revision_check CHECK ((definition_revision IS NULL OR definition_revision > 0)),
    CONSTRAINT book_preparation_failures_stage_check CHECK ((stage = ANY (ARRAY['entry'::text, 'catalog'::text, 'withdrawn'::text, 'fetch'::text, 'source'::text, 'import'::text]))),
    CONSTRAINT book_preparation_failures_code_check CHECK ((code ~ '^[a-z0-9_]+$'::text)),
    CONSTRAINT book_preparation_failures_unit_key_check CHECK ((reading_unit_key IS NULL OR btrim(reading_unit_key) <> ''::text)),
    CONSTRAINT book_preparation_failures_entry_pair_check CHECK (((connection_id IS NULL) = (entry_identifier IS NULL))),
    CONSTRAINT book_preparation_failures_entry_identifier_check CHECK ((entry_identifier IS NULL OR btrim(entry_identifier) <> ''::text)),
    CONSTRAINT book_preparation_failures_provenance_check CHECK ((((reading_unit_key IS NOT NULL) AND (definition_revision IS NOT NULL) AND (connection_id IS NULL) AND (entry_identifier IS NULL)) OR ((reading_unit_key IS NULL) AND (definition_revision IS NULL) AND (((connection_id IS NOT NULL) AND (entry_identifier IS NOT NULL)) OR ((connection_id IS NULL) AND (entry_identifier IS NULL) AND (stage = 'entry'::text)))))),
    CONSTRAINT book_preparation_failures_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT book_preparation_failures_unit_key_fkey FOREIGN KEY (reading_unit_key) REFERENCES public.reading_units(definition_key) ON DELETE RESTRICT,
    CONSTRAINT book_preparation_failures_owner_connection_fkey FOREIGN KEY (owner_id, connection_id) REFERENCES public.opds_connections(owner_id, id) ON DELETE CASCADE
);

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
